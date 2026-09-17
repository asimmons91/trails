package mail_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/require"
)

// welcomeEmail is exactly the shape an app mailer job takes: it embeds
// mail.Mailer by value and implements jobs.Job. No `json:"-"` tag anywhere.
type welcomeEmail struct {
	mail.Mailer
	UserEmail string
	UserName  string
}

func (j *welcomeEmail) Kind() string { return "mailers.welcome_email" }

func (j *welcomeEmail) Perform(ctx context.Context) error {
	return j.Deliver(ctx, "user_mailer/welcome_email", mail.Message{
		To:      []string{j.UserEmail},
		Subject: "Welcome!",
	}, map[string]any{"Name": j.UserName})
}

type fakeBackend struct {
	enqueued []jobs.Enqueued
}

func (b *fakeBackend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	b.enqueued = append(b.enqueued, e)
	return nil
}

func (b *fakeBackend) Close() error { return nil }

func TestMailerJobArgsCarryNoRendererOrTransportState(t *testing.T) {
	renderer := &fakeRenderer{html: "<p>hi</p>", text: "hi"}
	transport := &fakeTransport{}

	reg := jobs.NewRegistry()
	reg.Register("mailers.welcome_email", func() jobs.Job {
		return &welcomeEmail{Mailer: mail.NewMailer(renderer, transport, "from@example.com")}
	})

	backend := &fakeBackend{}
	job := &welcomeEmail{
		Mailer:    mail.NewMailer(renderer, transport, "from@example.com"),
		UserEmail: "user@example.com",
		UserName:  "Bob",
	}

	require.NoError(t, jobs.Enqueue(context.Background(), backend, job))
	require.Len(t, backend.enqueued, 1)

	// The serialized args are exactly the job's own fields — nothing from
	// the embedded Mailer leaked in.
	require.JSONEq(t, `{"UserEmail":"user@example.com","UserName":"Bob"}`, string(backend.enqueued[0].Args))

	// Dispatch builds a fresh job via the factory (which injects
	// renderer/transport) and unmarshals the args on top of it; the
	// injected dependencies must survive untouched.
	require.NoError(t, reg.Dispatch(context.Background(), backend.enqueued[0]))

	require.NotNil(t, transport.sent)
	require.Equal(t, []string{"user@example.com"}, transport.sent.To)
	require.Equal(t, "<p>hi</p>", transport.sent.HTML)
	require.Equal(t, "from@example.com", transport.sent.From)
}
