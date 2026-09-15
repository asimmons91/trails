package inline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/inline"
	"github.com/stretchr/testify/require"
)

type recordingJob struct {
	ran bool
}

func (j *recordingJob) Kind() string { return "recording" }

func (j *recordingJob) Perform(ctx context.Context) error {
	j.ran = true
	return nil
}

func TestBackendEnqueueRunsSynchronously(t *testing.T) {
	reg := jobs.NewRegistry()
	job := &recordingJob{}
	reg.Register("recording", func() jobs.Job { return job })

	b := inline.New(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), b, job))
	require.True(t, job.ran, "job should have run before Enqueue returned")
}

func TestBackendEnqueueReturnsPerformError(t *testing.T) {
	reg := jobs.NewRegistry()
	boom := errors.New("boom")
	reg.Register("failing", func() jobs.Job { return &failingJob{err: boom} })

	b := inline.New(reg)
	err := b.Enqueue(context.Background(), jobs.Enqueued{Kind: "failing"})
	require.ErrorIs(t, err, boom)
}

type failingJob struct{ err error }

func (j *failingJob) Kind() string                      { return "failing" }
func (j *failingJob) Perform(ctx context.Context) error { return j.err }

func TestBackendCloseIsNoop(t *testing.T) {
	b := inline.New(jobs.NewRegistry())
	require.NoError(t, b.Close())
}
