package mailtest

import (
	"context"
	"sync"

	"github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/assert"
)

type tHelper interface {
	Helper()
}

type Recorder struct {
	mu   sync.Mutex
	sent []*mail.Message
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) Send(ctx context.Context, msg *mail.Message) error {
	r.mu.Lock()
	r.sent = append(r.sent, msg)
	r.mu.Unlock()
	return nil
}

// Sent returns a snapshot of everything recorded so far.
func (r *Recorder) Sent() []*mail.Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]*mail.Message, len(r.sent))
	copy(out, r.sent)
	return out
}

// Reset clears the recording.
func (r *Recorder) Reset() {
	r.mu.Lock()
	r.sent = nil
	r.mu.Unlock()
}

// AssertSent fails t unless a recorded message satisfies predicate.
func AssertSent(t assert.TestingT, r *Recorder, predicate func(*mail.Message) bool) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	for _, msg := range r.Sent() {
		if predicate(msg) {
			return true
		}
	}

	return assert.Fail(t, "message not sent", "no recorded message matched the predicate")
}

// AssertSentTo fails t unless a recorded message was addressed to address.
func AssertSentTo(t assert.TestingT, r *Recorder, address string) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	for _, msg := range r.Sent() {
		for _, to := range msg.To {
			if to == address {
				return true
			}
		}
	}

	return assert.Fail(t, "message not sent to address", "expected a message sent to %q, but none was", address)
}

// AssertSentCount fails t unless exactly n messages were recorded.
func AssertSentCount(t assert.TestingT, r *Recorder, n int) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	return assert.Len(t, r.Sent(), n)
}

// AssertNoneSent fails t unless nothing was recorded.
func AssertNoneSent(t assert.TestingT, r *Recorder) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	return assert.Empty(t, r.Sent())
}
