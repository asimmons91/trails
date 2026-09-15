package jobstest_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/jobstest"
	"github.com/stretchr/testify/require"
)

type welcomeEmailJob struct {
	UserID uint
	ran    bool
}

func (j *welcomeEmailJob) Kind() string { return "welcome_email" }

func (j *welcomeEmailJob) Perform(ctx context.Context) error {
	j.ran = true
	return nil
}

func TestRecorderEnqueueRecordsWithoutRunning(t *testing.T) {
	reg := jobs.NewRegistry()
	job := &welcomeEmailJob{UserID: 42}
	reg.Register("welcome_email", func() jobs.Job { return job })

	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{UserID: 42}))

	require.False(t, job.ran, "recorder must not execute the job")
	require.Len(t, r.Jobs(), 1)
	require.Equal(t, "welcome_email", r.Jobs()[0].Kind)
}

func TestRecorderPerformEnqueuedRunsAndClearsRecording(t *testing.T) {
	reg := jobs.NewRegistry()
	job := &welcomeEmailJob{}
	reg.Register("welcome_email", func() jobs.Job { return job })

	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{UserID: 1}))

	require.NoError(t, r.PerformEnqueued(context.Background()))
	require.True(t, job.ran)
	require.Empty(t, r.Jobs())
}

func TestRecorderReset(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{}))

	r.Reset()

	require.Empty(t, r.Jobs())
}

func TestAssertEnqueuedPassesWhenKindPresent(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{UserID: 42}))

	require.True(t, jobstest.AssertEnqueued(t, r, "welcome_email"))
}

func TestAssertEnqueuedFailsWhenKindAbsent(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)

	spy := new(testingSpy)
	require.False(t, jobstest.AssertEnqueued(spy, r, "welcome_email"))
	require.True(t, spy.failed)
}

func TestAssertEnqueuedWithMatchesArgs(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{UserID: 42}))

	require.True(t, jobstest.AssertEnqueuedWith(t, r, "welcome_email", &welcomeEmailJob{UserID: 42}))
}

func TestAssertEnqueuedWithFailsOnMismatchedArgs(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{UserID: 42}))

	spy := new(testingSpy)
	require.False(t, jobstest.AssertEnqueuedWith(spy, r, "welcome_email", &welcomeEmailJob{UserID: 99}))
	require.True(t, spy.failed)
}

func TestAssertEnqueuedCount(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{}))
	require.NoError(t, jobs.Enqueue(context.Background(), r, &welcomeEmailJob{}))

	require.True(t, jobstest.AssertEnqueuedCount(t, r, 2))
}

func TestAssertNoneEnqueued(t *testing.T) {
	reg := jobs.NewRegistry()
	r := jobstest.NewRecorder(reg)

	require.True(t, jobstest.AssertNoneEnqueued(t, r))
}

// testingSpy is a minimal assert.TestingT stand-in so failure paths of the
// Assert* helpers can be verified without actually failing this test run.
type testingSpy struct {
	failed bool
}

func (s *testingSpy) Errorf(format string, args ...any) { s.failed = true }
