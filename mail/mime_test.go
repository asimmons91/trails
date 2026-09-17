package mail_test

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"

	trailsmail "github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/require"
)

func TestBuildRFC822WithBothPartsProducesMultipartAlternative(t *testing.T) {
	raw, err := trailsmail.BuildRFC822(&trailsmail.Message{
		From:    "from@example.com",
		To:      []string{"to1@example.com", "to2@example.com"},
		Subject: "Hi there",
		HTML:    "<p>hi</p>",
		Text:    "hi",
	})
	require.NoError(t, err)

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)

	require.Equal(t, "from@example.com", m.Header.Get("From"))
	require.Equal(t, "to1@example.com, to2@example.com", m.Header.Get("To"))
	require.Equal(t, "Hi there", m.Header.Get("Subject"))
	require.Equal(t, "1.0", m.Header.Get("MIME-Version"))

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/alternative", mediaType)

	mr := multipart.NewReader(m.Body, params["boundary"])

	part, err := mr.NextPart()
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=UTF-8", part.Header.Get("Content-Type"))
	data, err := io.ReadAll(part)
	require.NoError(t, err)
	require.Equal(t, "hi", string(data))

	part, err = mr.NextPart()
	require.NoError(t, err)
	require.Equal(t, "text/html; charset=UTF-8", part.Header.Get("Content-Type"))
	data, err = io.ReadAll(part)
	require.NoError(t, err)
	require.Equal(t, "<p>hi</p>", string(data))

	_, err = mr.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

func TestBuildRFC822HTMLOnlyIsSinglePart(t *testing.T) {
	raw, err := trailsmail.BuildRFC822(&trailsmail.Message{
		From: "from@example.com",
		To:   []string{"to@example.com"},
		HTML: "<p>hi</p>",
	})
	require.NoError(t, err)

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "text/html; charset=UTF-8", m.Header.Get("Content-Type"))

	body, err := io.ReadAll(m.Body)
	require.NoError(t, err)
	require.Equal(t, "<p>hi</p>", string(body))
}

func TestBuildRFC822TextOnlyIsSinglePart(t *testing.T) {
	raw, err := trailsmail.BuildRFC822(&trailsmail.Message{
		From: "from@example.com",
		To:   []string{"to@example.com"},
		Text: "hi",
	})
	require.NoError(t, err)

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=UTF-8", m.Header.Get("Content-Type"))

	body, err := io.ReadAll(m.Body)
	require.NoError(t, err)
	require.Equal(t, "hi", string(body))
}

func TestBuildRFC822ErrorsWithoutABody(t *testing.T) {
	_, err := trailsmail.BuildRFC822(&trailsmail.Message{From: "from@example.com"})
	require.Error(t, err)
}

func TestBuildRFC822IncludesCustomHeaders(t *testing.T) {
	raw, err := trailsmail.BuildRFC822(&trailsmail.Message{
		From:    "from@example.com",
		To:      []string{"to@example.com"},
		Text:    "hi",
		Headers: map[string]string{"X-App-Id": "trails"},
	})
	require.NoError(t, err)

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "trails", m.Header.Get("X-App-Id"))
}
