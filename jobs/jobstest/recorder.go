package jobstest

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/asimmons91/trails/jobs"
	"github.com/stretchr/testify/assert"
)

type tHelper interface {
	Helper()
}

type Recorder struct {
	reg *jobs.Registry

	mu       sync.Mutex
	enqueued []jobs.Enqueued
}

func NewRecorder(reg *jobs.Registry) *Recorder {
	return &Recorder{reg: reg}
}

func (r *Recorder) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	r.mu.Lock()
	r.enqueued = append(r.enqueued, e)
	r.mu.Unlock()
	return nil
}

func (r *Recorder) Close() error { return nil }

// Jobs returns a snapshot of everything recorded so far.
func (r *Recorder) Jobs() []jobs.Enqueued {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]jobs.Enqueued, len(r.enqueued))
	copy(out, r.enqueued)
	return out
}

// Reset clears the recording.
func (r *Recorder) Reset() {
	r.mu.Lock()
	r.enqueued = nil
	r.mu.Unlock()
}

// PerformEnqueued runs everything recorded so far through the Registry, in
// enqueue order, then clears the recording. It stops at the first error.
func (r *Recorder) PerformEnqueued(ctx context.Context) error {
	for _, e := range r.Jobs() {
		if err := r.reg.Dispatch(ctx, e); err != nil {
			return err
		}
	}
	r.Reset()
	return nil
}

// AssertEnqueued fails t unless a job of kind was recorded.
func AssertEnqueued(t assert.TestingT, r *Recorder, kind string) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	for _, e := range r.Jobs() {
		if e.Kind == kind {
			return true
		}
	}

	return assert.Fail(t, "job not enqueued", "expected a job of kind %q to be enqueued, but none was", kind)
}

// AssertEnqueuedWith fails t unless a job of kind was recorded whose
// arguments are semantically equal (via JSON) to args.
func AssertEnqueuedWith(t assert.TestingT, r *Recorder, kind string, args any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	expected, err := json.Marshal(args)
	if err != nil {
		return assert.Fail(t, "invalid expected args", "marshaling expected args for kind %q: %v", kind, err)
	}

	var expectedVal any
	if err := json.Unmarshal(expected, &expectedVal); err != nil {
		return assert.Fail(t, "invalid expected args", "unmarshaling expected args for kind %q: %v", kind, err)
	}

	for _, e := range r.Jobs() {
		if e.Kind != kind {
			continue
		}

		var actualVal any
		if err := json.Unmarshal(e.Args, &actualVal); err != nil {
			continue
		}

		if assert.ObjectsAreEqual(expectedVal, actualVal) {
			return true
		}
	}

	return assert.Fail(t, "job not enqueued with matching args", "expected a job of kind %q enqueued with args %s", kind, expected)
}

// AssertEnqueuedCount fails t unless exactly n jobs were recorded.
func AssertEnqueuedCount(t assert.TestingT, r *Recorder, n int) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	return assert.Len(t, r.Jobs(), n)
}

// AssertNoneEnqueued fails t unless nothing was recorded.
func AssertNoneEnqueued(t assert.TestingT, r *Recorder) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	return assert.Empty(t, r.Jobs())
}
