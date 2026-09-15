package cloudtask_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
)

func cleanupPushBody(scheduledFor int64) string {
	return fmt.Sprintf(`{"scheduled_for":%d}`, scheduledFor)
}

func TestHandleCleanupPush_MatchingScheduledFor_RunsCleanupAndReschedules(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	scheduledFor := time.Now().Add(-time.Minute).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{Name: "cleanup", NextRunAt: scheduledFor, CreatedAt: nowNanos()}))

	// A stale finished row that should be swept.
	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "finished",
		FinishedAt: time.Now().Add(-48 * time.Hour).UnixNano(), CreatedAt: nowNanos(),
	}))

	rec := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(scheduledFor))
	require.Equal(t, http.StatusOK, rec.Code)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Empty(t, rows, "stale finished row should have been swept")

	schedules, err := pack.Of[scheduleRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, schedules, 1)
	require.Greater(t, schedules[0].NextRunAt, time.Now().UnixNano(), "a successor cleanup should be scheduled")
	require.Equal(t, 1, client.countMatching(isCleanupRequest))
}

func TestHandleCleanupPush_MismatchedScheduledFor_NoOp(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	actual := time.Now().Add(time.Hour).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{Name: "cleanup", NextRunAt: actual, CreatedAt: nowNanos()}))

	// Simulate a duplicate/late delivery for a scheduled_for that no longer
	// matches the current authoritative row.
	rec := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(actual-1))
	require.Equal(t, http.StatusOK, rec.Code)

	require.Zero(t, client.countMatching(isCleanupRequest), "a mismatched delivery must not reschedule a second chain")

	rows, err := pack.Of[scheduleRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Equal(t, actual, rows[0].NextRunAt, "the authoritative row must be untouched by the no-op branch")
}

func TestHandleCleanupPush_DuplicateDelivery_DoesNotForkChain(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	scheduledFor := time.Now().Add(-time.Minute).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{Name: "cleanup", NextRunAt: scheduledFor, CreatedAt: nowNanos()}))

	rec1 := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(scheduledFor))
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, 1, client.countMatching(isCleanupRequest))

	// A second, duplicate delivery of the exact same (now-stale) scheduled_for.
	rec2 := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(scheduledFor))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, 1, client.countMatching(isCleanupRequest), "a duplicate delivery must not schedule a second successor")
}

func TestCleanupJobs_SweepsStaleFinishedAndFailedRows_KeepsRecentAndPending(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	old := time.Now().Add(-48 * time.Hour).UnixNano()
	recent := time.Now().Add(-time.Minute).UnixNano()

	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{Kind: "a", Args: "{}", Queue: "default", State: "finished", FinishedAt: old, CreatedAt: nowNanos()}))
	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{Kind: "b", Args: "{}", Queue: "default", State: "failed", FinishedAt: old, CreatedAt: nowNanos()}))
	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{Kind: "c", Args: "{}", Queue: "default", State: "finished", FinishedAt: recent, CreatedAt: nowNanos()}))
	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{Kind: "d", Args: "{}", Queue: "default", State: "pending", MaxAttempts: 5, CreatedAt: nowNanos()}))

	scheduledFor := time.Now().Add(-time.Minute).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{Name: "cleanup", NextRunAt: scheduledFor, CreatedAt: nowNanos()}))

	rec := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(scheduledFor))
	require.Equal(t, http.StatusOK, rec.Code)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	kinds := make(map[string]bool)
	for _, r := range rows {
		kinds[r.Kind] = true
	}
	require.False(t, kinds["a"], "old finished row should be swept")
	require.False(t, kinds["b"], "old failed row should be swept")
	require.True(t, kinds["c"], "recent finished row should be kept")
	require.True(t, kinds["d"], "pending row should never be swept")
}

func TestCleanupSlots_DeletesStaleSlotsWithoutActiveJob(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(), cloudtask.WithTokenVerifier(validVerifier()),
		cloudtask.WithSlotGracePeriod(time.Hour))
	mount := mountBackend(t, b)

	old := time.Now().Add(-2 * time.Hour).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &slotRowView{ConcurrencyKey: "idle", CreatedAt: old}))
	require.NoError(t, pack.Create(context.Background(), db, &slotRowView{ConcurrencyKey: "active", CreatedAt: old}))
	require.NoError(t, pack.Create(context.Background(), db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "executing",
		ConcurrencyKey: "active", LockedAt: nowNanos(), CreatedAt: nowNanos(),
	}))

	scheduledFor := time.Now().Add(-time.Minute).UnixNano()
	require.NoError(t, pack.Create(context.Background(), db, &scheduleRowView{Name: "cleanup", NextRunAt: scheduledFor, CreatedAt: nowNanos()}))

	rec := doPush(t, mount, "/tasks/cleanup", "tok", cleanupPushBody(scheduledFor))
	require.Equal(t, http.StatusOK, rec.Code)

	slots, err := pack.Of[slotRowView](db).Find(context.Background())
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, s := range slots {
		keys[s.ConcurrencyKey] = true
	}
	require.False(t, keys["idle"], "a stale slot with no active job should be deleted")
	require.True(t, keys["active"], "a stale slot backing a currently-executing job must be kept")
}
