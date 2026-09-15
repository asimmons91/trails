package cloudtask

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/asimmons91/trails/pack"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const scheduleNameCleanup = "cleanup"

const scheduleClaimedSentinel = int64(math.MaxInt64)

type scheduleRow struct {
	pack.Model[int64] `db:"table:cloudtask_schedule"`

	Name      string `db:"name,not_null"` // unique index
	NextRunAt int64  `db:"next_run_at,not_null"`
	CreatedAt int64  `db:"created_at,not_null"`
}

var scheduleCol = struct {
	ID        pack.Col[scheduleRow, int64]
	Name      pack.Col[scheduleRow, string]
	NextRunAt pack.Col[scheduleRow, int64]
	CreatedAt pack.Col[scheduleRow, int64]
}{
	ID:        pack.Field(func(r *scheduleRow) *int64 { return &r.ID }),
	Name:      pack.Field(func(r *scheduleRow) *string { return &r.Name }),
	NextRunAt: pack.Field(func(r *scheduleRow) *int64 { return &r.NextRunAt }),
	CreatedAt: pack.Field(func(r *scheduleRow) *int64 { return &r.CreatedAt }),
}

type cleanupPayload struct {
	ScheduledFor int64 `json:"scheduled_for"`
}

func (b *Backend) ensureScheduled(ctx context.Context) {
	now := time.Now().UnixNano()
	var nextRunAt int64
	claimed := false

	err := b.db.Tx(ctx, func(tx *pack.DB) error {
		if err := pack.Create(ctx, tx, &scheduleRow{Name: scheduleNameCleanup, NextRunAt: 0, CreatedAt: now},
			pack.OnConflict[scheduleRow](scheduleCol.Name).DoNothing()); err != nil {
			return fmt.Errorf("cloudtask: upsert cleanup schedule row: %w", err)
		}

		q := pack.Of[scheduleRow](tx).Where(scheduleCol.Name.Eq(scheduleNameCleanup))
		if tx.Dialect().SupportsRowLocking() {
			q = q.ForUpdate()
		}
		row, err := q.First(ctx)
		if err != nil {
			return fmt.Errorf("cloudtask: lock cleanup schedule row: %w", err)
		}

		if row.NextRunAt > now {
			return nil // already scheduled ahead of time; nothing to do
		}

		// First run ever, or the chain went stale/broke. Claim responsibility
		// by advancing NextRunAt before calling CreateTask, so a concurrent
		// Enqueue racing on the same stale row doesn't also try to bootstrap.
		nextRunAt = now + b.cleanupInterval.Nanoseconds()
		if _, err := pack.Of[scheduleRow](tx).Where(scheduleCol.ID.Eq(row.ID)).Update(ctx,
			scheduleCol.NextRunAt.Set(nextRunAt),
		); err != nil {
			return fmt.Errorf("cloudtask: claim cleanup schedule bootstrap: %w", err)
		}
		claimed = true
		return nil
	})
	if err != nil {
		b.logger.Error("cloudtask: ensure cleanup schedule", "error", err)
		return
	}
	if !claimed {
		return
	}

	if err := b.scheduleCleanupTask(ctx, nextRunAt); err != nil {
		b.logger.Error("cloudtask: schedule cleanup task", "error", err)
		// The next Enqueue's ensureScheduled call should retry immediately
		// rather than waiting a full cleanupInterval for this row to go
		// stale again.
		if _, resetErr := pack.Of[scheduleRow](b.db).Where(scheduleCol.Name.Eq(scheduleNameCleanup)).Update(ctx,
			scheduleCol.NextRunAt.Set(int64(0)),
		); resetErr != nil {
			b.logger.Error("cloudtask: reset cleanup schedule after failed bootstrap", "error", resetErr)
		}
	}
}

func (b *Backend) scheduleCleanupTask(ctx context.Context, scheduledFor int64) error {
	body, err := json.Marshal(cleanupPayload{ScheduledFor: scheduledFor})
	if err != nil {
		return fmt.Errorf("cloudtask: marshal cleanup payload: %w", err)
	}

	_, err = b.client.CreateTask(ctx, &cloudtaskspb.CreateTaskRequest{
		Parent: b.queuePath(defaultQueue),
		Task: &cloudtaskspb.Task{
			MessageType: &cloudtaskspb.Task_HttpRequest{HttpRequest: &cloudtaskspb.HttpRequest{
				Url:        b.cleanupURL(),
				HttpMethod: cloudtaskspb.HttpMethod_POST,
				Headers:    map[string]string{"Content-Type": "application/json"},
				Body:       body,
				AuthorizationHeader: &cloudtaskspb.HttpRequest_OidcToken{OidcToken: &cloudtaskspb.OidcToken{
					ServiceAccountEmail: b.cfg.ServiceAccountEmail,
					Audience:            b.cfg.Audience,
				}},
			}},
			ScheduleTime: timestamppb.New(time.Unix(0, scheduledFor)),
		},
	})
	return err
}
