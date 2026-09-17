package mail_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/require"
)

func TestLogTransportLogsInsteadOfSending(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	transport := mail.NewLogTransport(logger)

	err := transport.Send(context.Background(), &mail.Message{
		From:    "from@example.com",
		To:      []string{"to@example.com"},
		Subject: "Hi",
		HTML:    "<p>hi</p>",
	})
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "from@example.com")
	require.Contains(t, out, "to@example.com")
	require.Contains(t, out, "Hi")
}
