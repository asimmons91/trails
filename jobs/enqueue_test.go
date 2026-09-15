package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeBackend struct {
	enqueued []Enqueued
}

func (b *fakeBackend) Enqueue(ctx context.Context, e Enqueued) error {
	b.enqueued = append(b.enqueued, e)
	return nil
}

func (b *fakeBackend) Close() error { return nil }

type plainJob struct {
	UserID uint
}

func (j *plainJob) Kind() string                      { return "plain" }
func (j *plainJob) Perform(ctx context.Context) error { return nil }

type limitedJob struct {
	plainJob
}

func (j *limitedJob) Kind() string { return "limited" }

func (j *limitedJob) ConcurrencyKey() string { return "shared-key" }

func (j *limitedJob) ConcurrencyLimit() (int, time.Duration) { return 1, time.Minute }

func TestEnqueueMarshalsJobAsArgs(t *testing.T) {
	b := &fakeBackend{}

	err := Enqueue(context.Background(), b, &plainJob{UserID: 7})
	require.NoError(t, err)
	require.Len(t, b.enqueued, 1)
	require.Equal(t, "plain", b.enqueued[0].Kind)

	var args plainJob
	require.NoError(t, json.Unmarshal(b.enqueued[0].Args, &args))
	require.Equal(t, uint(7), args.UserID)
}

func TestEnqueueAppliesDefaultMaxAttempts(t *testing.T) {
	b := &fakeBackend{}

	require.NoError(t, Enqueue(context.Background(), b, &plainJob{}))
	require.Equal(t, DefaultMaxAttempts, b.enqueued[0].MaxAttempts)
}

func TestEnqueueOptionsOverrideDefaults(t *testing.T) {
	b := &fakeBackend{}

	require.NoError(t, Enqueue(context.Background(), b, &plainJob{}, WithMaxAttempts(3), WithQueue("mailers")))
	require.Equal(t, 3, b.enqueued[0].MaxAttempts)
	require.Equal(t, "mailers", b.enqueued[0].Queue)
}

func TestWithDelaySetsFutureScheduledAt(t *testing.T) {
	b := &fakeBackend{}
	before := time.Now()

	require.NoError(t, Enqueue(context.Background(), b, &plainJob{}, WithDelay(time.Hour)))

	require.True(t, b.enqueued[0].ScheduledAt.After(before.Add(59*time.Minute)))
}

func TestEnqueuePopulatesConcurrencyFieldsFromJob(t *testing.T) {
	b := &fakeBackend{}

	require.NoError(t, Enqueue(context.Background(), b, &limitedJob{}))

	e := b.enqueued[0]
	require.Equal(t, "shared-key", e.ConcurrencyKey)
	require.Equal(t, 1, e.ConcurrencyLimit)
	require.Equal(t, time.Minute, e.ConcurrencyDuration)
}

func TestEnqueueLeavesConcurrencyFieldsZeroWhenJobNotLimited(t *testing.T) {
	b := &fakeBackend{}

	require.NoError(t, Enqueue(context.Background(), b, &plainJob{}))

	e := b.enqueued[0]
	require.Equal(t, "", e.ConcurrencyKey)
	require.Equal(t, 0, e.ConcurrencyLimit)
}
