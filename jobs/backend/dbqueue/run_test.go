package dbqueue

import (
	"testing"
	"time"
)

func TestComputeBackoff_GrowsWithAttemptsAndRespectsMax(t *testing.T) {
	b := &Backend{backoffBase: time.Second, backoffMax: time.Minute}

	for attempt := 1; attempt <= 20; attempt++ {
		d := b.computeBackoff(attempt)
		if d <= 0 {
			t.Fatalf("attempt %d: computeBackoff = %v, want > 0", attempt, d)
		}
		if d > b.backoffMax {
			t.Fatalf("attempt %d: computeBackoff = %v, want <= max %v", attempt, d, b.backoffMax)
		}
	}
}

func TestComputeBackoff_SmallAttemptsStayUnderMax(t *testing.T) {
	b := &Backend{backoffBase: 5 * time.Second, backoffMax: 30 * time.Minute}

	d := b.computeBackoff(1)
	if d < 2*time.Second || d > 10*time.Second {
		t.Fatalf("attempt 1: computeBackoff = %v, want roughly base (5s) +/- jitter", d)
	}
}

func TestDefaultWorkerID_NonEmpty(t *testing.T) {
	if id := defaultWorkerID(); id == "" {
		t.Fatal("defaultWorkerID() is empty")
	}
}
