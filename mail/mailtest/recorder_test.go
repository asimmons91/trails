package mailtest_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/mail"
	"github.com/asimmons91/trails/mail/mailtest"
	"github.com/stretchr/testify/require"
)

func TestRecorderSendRecordsMessage(t *testing.T) {
	r := mailtest.NewRecorder()

	require.NoError(t, r.Send(context.Background(), &mail.Message{To: []string{"a@example.com"}, Subject: "Hi"}))

	require.Len(t, r.Sent(), 1)
	require.Equal(t, "Hi", r.Sent()[0].Subject)
}

func TestRecorderReset(t *testing.T) {
	r := mailtest.NewRecorder()
	require.NoError(t, r.Send(context.Background(), &mail.Message{}))

	r.Reset()

	require.Empty(t, r.Sent())
}

func TestAssertSentPassesWhenPredicateMatches(t *testing.T) {
	r := mailtest.NewRecorder()
	require.NoError(t, r.Send(context.Background(), &mail.Message{Subject: "Welcome"}))

	require.True(t, mailtest.AssertSent(t, r, func(m *mail.Message) bool { return m.Subject == "Welcome" }))
}

func TestAssertSentFailsWhenPredicateNeverMatches(t *testing.T) {
	r := mailtest.NewRecorder()

	spy := new(testingSpy)
	require.False(t, mailtest.AssertSent(spy, r, func(m *mail.Message) bool { return false }))
	require.True(t, spy.failed)
}

func TestAssertSentToPassesWhenAddressPresent(t *testing.T) {
	r := mailtest.NewRecorder()
	require.NoError(t, r.Send(context.Background(), &mail.Message{To: []string{"user@example.com"}}))

	require.True(t, mailtest.AssertSentTo(t, r, "user@example.com"))
}

func TestAssertSentToFailsWhenAddressAbsent(t *testing.T) {
	r := mailtest.NewRecorder()

	spy := new(testingSpy)
	require.False(t, mailtest.AssertSentTo(spy, r, "user@example.com"))
	require.True(t, spy.failed)
}

func TestAssertSentCount(t *testing.T) {
	r := mailtest.NewRecorder()
	require.NoError(t, r.Send(context.Background(), &mail.Message{}))
	require.NoError(t, r.Send(context.Background(), &mail.Message{}))

	require.True(t, mailtest.AssertSentCount(t, r, 2))
}

func TestAssertNoneSent(t *testing.T) {
	r := mailtest.NewRecorder()

	require.True(t, mailtest.AssertNoneSent(t, r))
}

// testingSpy is a minimal assert.TestingT stand-in so failure paths of the
// Assert* helpers can be verified without actually failing this test run.
type testingSpy struct {
	failed bool
}

func (s *testingSpy) Errorf(format string, args ...any) { s.failed = true }
