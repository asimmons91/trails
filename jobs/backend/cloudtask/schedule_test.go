package cloudtask_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
)

// ensureScheduled itself is unexported, so these exercise it the way real
// callers do: through Enqueue, which calls it at the end of every call.

func TestEnqueue_BootstrapsCleanupScheduleOnFirstCall(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))

	rows, err := pack.Of[scheduleRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "cleanup", rows[0].Name)
	require.Greater(t, rows[0].NextRunAt, time.Now().UnixNano())

	require.Equal(t, 1, client.countMatching(isCleanupRequest), "exactly one cleanup task should be scheduled")

	req := client.requests[len(client.requests)-1]
	require.True(t, isCleanupRequest(req))
	var payload struct {
		ScheduledFor int64 `json:"scheduled_for"`
	}
	require.NoError(t, json.Unmarshal(req.Task.GetHttpRequest().Body, &payload))
	require.Equal(t, rows[0].NextRunAt, payload.ScheduledFor)
}

func TestEnqueue_DoesNotRescheduleWhenChainAlreadyRunning(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))
	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))

	require.Equal(t, 1, client.countMatching(isCleanupRequest), "a schedule already ahead of time should not be re-bootstrapped")
	require.Equal(t, 2, client.countMatching(func(r *cloudtaskspb.CreateTaskRequest) bool { return !isCleanupRequest(r) }))
}

func TestEnqueue_HealsStaleSchedule(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	// Simulate a chain that broke: a schedule row exists but its NextRunAt
	// is in the past (as if a prior reschedule attempt failed).
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{
		Name: "cleanup", NextRunAt: time.Now().Add(-time.Hour).UnixNano(), CreatedAt: nowNanos(),
	}))

	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))

	require.Equal(t, 1, client.countMatching(isCleanupRequest), "a stale schedule should be healed")

	rows, err := pack.Of[scheduleRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Greater(t, rows[0].NextRunAt, time.Now().UnixNano())
}

func TestEnqueue_ResetsScheduleWhenCleanupTaskCreationFails(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{failWhen: isCleanupRequest}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	// The job's own task still succeeds; only the cleanup bootstrap fails.
	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))

	rows, err := pack.Of[scheduleRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Zero(t, rows[0].NextRunAt, "a failed bootstrap must reset NextRunAt so the next Enqueue retries immediately")

	// The next Enqueue call should retry the bootstrap.
	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))
	require.Equal(t, 2, client.countMatching(isCleanupRequest), "the retried bootstrap should attempt CreateTask again")
}
