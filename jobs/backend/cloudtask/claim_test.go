package cloudtask_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
)

func TestHandlePush_ExhaustedBeforeClaim_AcksWithoutDispatch(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var ran atomic.Bool
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { ran.Store(true); return nil }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	insertJobRow(t, db, &jobRowView{Kind: "k", Args: "{}", Queue: "default", State: "pending", Attempts: 3, MaxAttempts: 3})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, ran.Load(), "a job whose attempts are already exhausted must not be dispatched")
}

func TestHandlePush_AlreadyExecuting_SkipsAsNoOp(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var ran atomic.Bool
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { ran.Store(true); return nil }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	insertJobRow(t, db, &jobRowView{Kind: "k", Args: "{}", Queue: "default", State: "executing", Attempts: 1, MaxAttempts: 5, LockedAt: nowNanos()})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, ran.Load(), "a duplicate delivery for an already-executing row must not dispatch again")
}

func TestHandlePush_ConcurrencyWindowExpired_AllowsExecution(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var ran atomic.Bool
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { ran.Store(true); return nil }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	// A previous "executing" row for this key whose lock window has long
	// since expired must not count against the limit.
	insertJobRow(t, db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "executing",
		ConcurrencyKey: "shared", ConcurrencyLimit: 1, ConcurrencyDurationNs: int64(time.Minute),
		LockedAt: time.Now().Add(-time.Hour).UnixNano(),
	})
	insertJobRow(t, db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "pending", MaxAttempts: 5,
		ConcurrencyKey: "shared", ConcurrencyLimit: 1, ConcurrencyDurationNs: int64(time.Minute),
	})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":2}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ran.Load(), "a job should be claimable once the prior holder's concurrency window has expired")
}

// TestHandlePush_ConcurrencyGuard_RaceOverBrandNewKey is the regression test
// the slot-row design in slot.go/claim.go exists for: many independent HTTP
// push requests racing to claim jobs that all share one ConcurrencyKey no
// row has ever used before, so there is no pre-existing jobRow a naive
// COUNT-then-act check could lock. Without the upsert-then-FOR UPDATE slot,
// this is exactly the check-then-insert race where multiple "first" claims
// could all pass the count check before any of them commit.
func TestHandlePush_ConcurrencyGuard_RaceOverBrandNewKey(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()

	const limit = 3
	var current, max atomic.Int32
	release := make(chan struct{})
	reg.Register("race", func() jobs.Job {
		return &testJob{kind: "race", perform: func(context.Context) error {
			now := current.Add(1)
			for {
				m := max.Load()
				if now <= m || max.CompareAndSwap(m, now) {
					break
				}
			}
			<-release
			current.Add(-1)
			return nil
		}}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	const n = 20
	for range n {
		insertJobRow(t, db, &jobRowView{
			Kind: "race", Args: "{}", Queue: "default", State: "pending", MaxAttempts: 5,
			ConcurrencyKey: "brand-new-key", ConcurrencyLimit: limit, ConcurrencyDurationNs: int64(time.Minute),
		})
	}

	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := doPush(t, mount, "/tasks/run", "tok", fmt.Sprintf(`{"job_id":%d}`, i+1))
			codes[i] = rec.Code
		}(i)
	}

	require.Eventually(t, func() bool { return current.Load() > 0 }, 2*time.Second, 5*time.Millisecond,
		"at least one job should have started executing")
	time.Sleep(150 * time.Millisecond) // let every claim attempt reach the guard before releasing any of them
	close(release)
	wg.Wait()

	require.LessOrEqual(t, int(max.Load()), limit, "no more than the concurrency limit should ever execute at once, even racing over a brand-new key")

	executed, deferred := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			executed++
		case http.StatusTooManyRequests:
			deferred++
		default:
			t.Fatalf("unexpected status code %d", code)
		}
	}
	require.Equal(t, n, executed+deferred)
	require.GreaterOrEqual(t, executed, 1)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	for _, row := range rows {
		if row.State == "pending" {
			require.Zero(t, row.Attempts, "a deferred claim must not consume an attempt")
		}
	}
}
