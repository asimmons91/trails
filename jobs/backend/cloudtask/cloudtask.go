package cloudtask

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/pack"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultPushPath        = "/tasks/run"
	defaultCleanupPath     = "/tasks/cleanup"
	defaultRetention       = 24 * time.Hour
	defaultCleanupInterval = 5 * time.Minute
	defaultSlotGrace       = time.Hour
)

// Config holds the GCP project/queue and callback details a host app must
// supply — this backend never provisions any of it itself.
type Config struct {
	// ProjectID and Location identify the GCP project/region whose Cloud
	// Tasks queues Enqueue targets. Queues themselves must already exist
	// (e.g. via `gcloud tasks queues create` or Terraform); this backend
	// only ever maps a logical queue name to a queue resource path, it
	// never creates one.
	ProjectID string
	Location  string

	// BaseURL is the publicly reachable scheme+host+mount-prefix Cloud
	// Tasks calls back into, e.g. "https://myapp-abc123-uc.a.run.app/cloudtasks"
	// (no trailing slash). It must match the prefix this Spur is mounted
	// under in the host app's router — Routes only knows its relative
	// path, not the app's external hostname, so the host must supply this
	// explicitly and keep the two in sync.
	BaseURL string

	// ServiceAccountEmail is used both to mint the OIDC token Cloud Tasks
	// attaches to each push (HttpRequest.OidcToken) and, on receipt, to
	// authorize it (the push handler rejects any token whose `email` claim
	// doesn't match).
	ServiceAccountEmail string

	// Audience is the OIDC audience Cloud Tasks mints the token for and the
	// push handler verifies against. Defaults to BaseURL.
	Audience string
}

type Option func(*Backend)

func WithPushPath(p string) Option { return func(b *Backend) { b.pushPath = p } }

func WithCleanupPath(p string) Option { return func(b *Backend) { b.cleanupPath = p } }

// WithQueueName maps a logical jobs.Enqueued.Queue name ("" included) to a
// Cloud Tasks queue ID. The default maps "" to "default" and otherwise
// passes the name through unchanged.
func WithQueueName(fn func(logical string) string) Option {
	return func(b *Backend) { b.queueName = fn }
}

func WithRetention(d time.Duration) Option { return func(b *Backend) { b.retention = d } }

func WithCleanupInterval(d time.Duration) Option { return func(b *Backend) { b.cleanupInterval = d } }

func WithSlotGracePeriod(d time.Duration) Option { return func(b *Backend) { b.slotGrace = d } }

func WithLogger(l *slog.Logger) Option { return func(b *Backend) { b.logger = l } }

func WithTokenVerifier(v TokenVerifier) Option { return func(b *Backend) { b.verifier = v } }

func WithDispatchDeadline(d time.Duration) Option {
	return func(b *Backend) { b.dispatchDeadline = d }
}

var _ jobs.Backend = (*Backend)(nil)

type Backend struct {
	db     *pack.DB
	reg    *jobs.Registry
	client TaskClient
	cfg    Config

	pushPath         string
	cleanupPath      string
	queueName        func(string) string
	retention        time.Duration
	cleanupInterval  time.Duration
	slotGrace        time.Duration
	logger           *slog.Logger
	verifier         TokenVerifier
	dispatchDeadline time.Duration
}

func New(db *pack.DB, reg *jobs.Registry, client TaskClient, cfg Config, opts ...Option) *Backend {
	if db == nil || reg == nil || client == nil {
		panic("cloudtask: db, reg, and client are required")
	}
	if cfg.ProjectID == "" || cfg.Location == "" || cfg.BaseURL == "" || cfg.ServiceAccountEmail == "" {
		panic("cloudtask: Config.ProjectID, Location, BaseURL, and ServiceAccountEmail are required")
	}
	if cfg.Audience == "" {
		cfg.Audience = cfg.BaseURL
	}

	b := &Backend{
		db:              db,
		reg:             reg,
		client:          client,
		cfg:             cfg,
		pushPath:        defaultPushPath,
		cleanupPath:     defaultCleanupPath,
		queueName:       defaultQueueName,
		retention:       defaultRetention,
		cleanupInterval: defaultCleanupInterval,
		slotGrace:       defaultSlotGrace,
		logger:          slog.Default(),
		verifier:        defaultVerifier{},
	}
	for _, opt := range opts {
		opt(b)
	}

	return b
}

func defaultQueueName(logical string) string {
	if logical == "" {
		return defaultQueue
	}
	return logical
}

func (b *Backend) queuePath(logical string) string {
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", b.cfg.ProjectID, b.cfg.Location, b.queueName(logical))
}

func (b *Backend) pushURL() string    { return b.cfg.BaseURL + b.pushPath }
func (b *Backend) cleanupURL() string { return b.cfg.BaseURL + b.cleanupPath }

func (b *Backend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	queue := e.Queue
	if queue == "" {
		queue = defaultQueue
	}

	maxAttempts := e.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = jobs.DefaultMaxAttempts
	}

	row := &jobRow{
		Kind:                  e.Kind,
		Args:                  string(e.Args),
		Queue:                 queue,
		State:                 statePending,
		MaxAttempts:           maxAttempts,
		ConcurrencyKey:        e.ConcurrencyKey,
		ConcurrencyLimit:      e.ConcurrencyLimit,
		ConcurrencyDurationNs: int64(e.ConcurrencyDuration),
		CreatedAt:             time.Now().UnixNano(),
	}
	if err := pack.Create(ctx, b.db, row); err != nil {
		return fmt.Errorf("cloudtask: enqueue kind %q: %w", e.Kind, err)
	}

	body, err := json.Marshal(pushPayload{JobID: row.ID})
	if err != nil {
		return fmt.Errorf("cloudtask: marshal push payload: %w", err)
	}

	task := &cloudtaskspb.Task{
		MessageType: &cloudtaskspb.Task_HttpRequest{HttpRequest: &cloudtaskspb.HttpRequest{
			Url:        b.pushURL(),
			HttpMethod: cloudtaskspb.HttpMethod_POST,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
			AuthorizationHeader: &cloudtaskspb.HttpRequest_OidcToken{OidcToken: &cloudtaskspb.OidcToken{
				ServiceAccountEmail: b.cfg.ServiceAccountEmail,
				Audience:            b.cfg.Audience,
			}},
		}},
	}
	if !e.ScheduledAt.IsZero() {
		task.ScheduleTime = timestamppb.New(e.ScheduledAt)
	}
	if b.dispatchDeadline > 0 {
		task.DispatchDeadline = durationpb.New(b.dispatchDeadline)
	}

	resp, err := b.client.CreateTask(ctx, &cloudtaskspb.CreateTaskRequest{
		Parent: b.queuePath(queue),
		Task:   task,
	})
	if err != nil {
		if _, delErr := pack.Of[jobRow](b.db).Where(jobCol.ID.Eq(row.ID)).Delete(ctx); delErr != nil {
			b.logger.Error("cloudtask: cleanup orphaned row after CreateTask failure", "id", row.ID, "error", delErr)
		}
		return fmt.Errorf("cloudtask: create task for kind %q: %w", e.Kind, err)
	}

	if _, err := pack.Of[jobRow](b.db).Where(jobCol.ID.Eq(row.ID)).Update(ctx, jobCol.TaskName.Set(resp.Name)); err != nil {
		b.logger.Warn("cloudtask: record task name", "id", row.ID, "error", err)
	}

	b.ensureScheduled(ctx)

	return nil
}

// Close is a no-op: there is no in-process goroutine or open resource this
// backend owns. Satisfies jobs.Backend.
func (b *Backend) Close() error { return nil }
