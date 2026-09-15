package cloudtask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs"
)

type pushPayload struct {
	JobID int64 `json:"job_id"`
}

var (
	errMissingToken        = errors.New("cloudtask: missing bearer token")
	errInvalidToken        = errors.New("cloudtask: invalid token")
	errWrongServiceAccount = errors.New("cloudtask: unexpected service account")
)

// authenticate verifies the OIDC bearer token Cloud Tasks attaches to a
// push request, shared by both handlePush and handleCleanupPush.
func (b *Backend) authenticate(ctx context.Context, r *http.Request) error {
	token, err := bearerToken(r)
	if err != nil {
		return errMissingToken
	}

	payload, err := b.verifier.Validate(ctx, token, b.cfg.Audience)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidToken, err)
	}

	email, _ := payload.Claims["email"].(string)
	if email != b.cfg.ServiceAccountEmail {
		return fmt.Errorf("%w: %q", errWrongServiceAccount, email)
	}

	return nil
}

func bearerToken(r *http.Request) (string, error) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", errors.New("cloudtask: missing bearer token")
	}
	return strings.TrimPrefix(h, prefix), nil
}

// handlePush is the Cloud Tasks push target for job execution, mounted by
// Routes at pushPath.
//
// Response codes:
//
//	missing/invalid bearer token          401   Cloud Tasks retries per queue config
//	valid token, wrong service account    403   Cloud Tasks retries per queue config
//	malformed body                        400   Cloud Tasks retries per queue config
//	row already non-pending, or exhausted 200   task removed
//	concurrency limit currently full      429   Cloud Tasks retries per queue config
//	Perform succeeded                     200   task removed
//	Perform failed, attempts remain       500   Cloud Tasks retries per queue config
//	Perform failed, attempts exhausted    200   task removed (logged as permanent failure)
//
// Cloud Tasks' RetryConfig is configured once per queue, not per task, so
// it can't be made to exactly track an arbitrary job's MaxAttempts. This
// handler tracks Attempts locally (claim.go/finish.go) and always acks
// (200) once the job's own MaxAttempts is exhausted, terminating the task
// even if the queue's RetryConfig would otherwise keep retrying it. A job
// needing different backoff/attempt behavior than its default queue
// provides should be routed to a distinct Cloud Tasks queue (Enqueued.Queue
// -> WithQueueName) provisioned with different retry config — this backend
// deliberately does not reimplement dbqueue's computeBackoff; Cloud Tasks
// owns retry timing.
func (b *Backend) handlePush(c *trails.Context) error {
	ctx := c.Request().Context()

	if err := b.authenticate(ctx, c.Request()); err != nil {
		if errors.Is(err, errWrongServiceAccount) {
			c.Logger().Warn("cloudtask: reject push, unexpected service account", "error", err)
			return c.String(http.StatusForbidden, "forbidden")
		}
		c.Logger().Warn("cloudtask: reject push, invalid token", "error", err)
		return c.String(http.StatusUnauthorized, "unauthorized")
	}

	var body pushPayload
	if err := c.Bind(&body); err != nil || body.JobID == 0 {
		return c.String(http.StatusBadRequest, "bad request")
	}

	row, result, err := b.claimForExecution(ctx, body.JobID)
	if err != nil {
		c.Logger().Error("cloudtask: claim failed", "job_id", body.JobID, "error", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}

	switch result {
	case claimSkip, claimExhausted:
		return c.String(http.StatusOK, "ok")
	case claimDeferred:
		return c.String(http.StatusTooManyRequests, "concurrency limit reached")
	}

	perr := b.reg.Dispatch(ctx, jobs.Enqueued{Kind: row.Kind, Args: json.RawMessage(row.Args)})
	if perr == nil {
		if err := b.finish(ctx, row.ID); err != nil {
			c.Logger().Error("cloudtask: mark finished", "job_id", row.ID, "error", err)
		}
		return c.String(http.StatusOK, "ok")
	}

	exhausted, ferr := b.fail(ctx, row, perr)
	if ferr != nil {
		c.Logger().Error("cloudtask: record failure", "job_id", row.ID, "error", ferr)
	}
	if exhausted {
		c.Logger().Error("cloudtask: job permanently failed", "job_id", row.ID, "kind", row.Kind, "attempts", row.Attempts, "error", perr)
		return c.String(http.StatusOK, "ok")
	}

	c.Logger().Error("cloudtask: job failed, will retry", "job_id", row.ID, "kind", row.Kind, "attempts", row.Attempts, "error", perr)
	return c.String(http.StatusInternalServerError, "job failed")
}
