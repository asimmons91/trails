package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOptions_OverrideDefaults(t *testing.T) {
	b := &Backend{}
	opts := []Option{
		WithPollInterval(10 * time.Millisecond),
		WithBatchSize(5),
		WithRetention(time.Hour),
		WithTrimInterval(30 * time.Second),
		WithTrimBatchSize(50),
	}
	for _, opt := range opts {
		opt(b)
	}

	require.Equal(t, 10*time.Millisecond, b.pollInterval)
	require.Equal(t, 5, b.batchSize)
	require.Equal(t, time.Hour, b.retention)
	require.Equal(t, 30*time.Second, b.trimInterval)
	require.Equal(t, 50, b.trimBatchSize)
}
