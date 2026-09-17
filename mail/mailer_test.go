package mail_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/require"
)

type fakeRenderer struct {
	html, text string
	err        error
}

func (f *fakeRenderer) Render(name string, data any) (string, string, error) {
	return f.html, f.text, f.err
}

type fakeTransport struct {
	sent *mail.Message
	err  error
}

func (f *fakeTransport) Send(ctx context.Context, msg *mail.Message) error {
	f.sent = msg
	return f.err
}

func TestDeliverRendersAndSends(t *testing.T) {
	renderer := &fakeRenderer{html: "<p>hi</p>", text: "hi"}
	transport := &fakeTransport{}
	m := mail.NewMailer(renderer, transport, "from@example.com")

	err := m.Deliver(context.Background(), "user_mailer/welcome_email", mail.Message{
		To:      []string{"user@example.com"},
		Subject: "Hi",
	}, nil)
	require.NoError(t, err)

	require.NotNil(t, transport.sent)
	require.Equal(t, "from@example.com", transport.sent.From)
	require.Equal(t, "<p>hi</p>", transport.sent.HTML)
	require.Equal(t, "hi", transport.sent.Text)
	require.Equal(t, "Hi", transport.sent.Subject)
}

func TestDeliverKeepsExplicitFrom(t *testing.T) {
	transport := &fakeTransport{}
	m := mail.NewMailer(&fakeRenderer{}, transport, "default@example.com")

	require.NoError(t, m.Deliver(context.Background(), "x", mail.Message{From: "explicit@example.com"}, nil))
	require.Equal(t, "explicit@example.com", transport.sent.From)
}

func TestDeliverReturnsRenderError(t *testing.T) {
	wantErr := errors.New("boom")
	m := mail.NewMailer(&fakeRenderer{err: wantErr}, &fakeTransport{}, "")

	err := m.Deliver(context.Background(), "x", mail.Message{}, nil)
	require.ErrorIs(t, err, wantErr)
}

func TestDeliverReturnsSendError(t *testing.T) {
	wantErr := errors.New("boom")
	m := mail.NewMailer(&fakeRenderer{}, &fakeTransport{err: wantErr}, "")

	err := m.Deliver(context.Background(), "x", mail.Message{}, nil)
	require.ErrorIs(t, err, wantErr)
}
