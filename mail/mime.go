package mail

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// BuildRFC822 encodes msg into a raw RFC 822 message: multipart/alternative
// when both HTML and Text are set, a single text/html or text/plain part
// when only one is, and an error when neither is set. Headers are written
// in sorted order for deterministic output.
func BuildRFC822(msg *Message) ([]byte, error) {
	if msg.HTML == "" && msg.Text == "" {
		return nil, fmt.Errorf("mail: message has neither an HTML nor a text body")
	}

	body, contentType, err := buildBody(msg)
	if err != nil {
		return nil, err
	}

	header := make(textproto.MIMEHeader)
	header.Set("From", msg.From)
	if len(msg.To) > 0 {
		header.Set("To", strings.Join(msg.To, ", "))
	}
	if len(msg.Cc) > 0 {
		header.Set("Cc", strings.Join(msg.Cc, ", "))
	}
	if msg.ReplyTo != "" {
		header.Set("Reply-To", msg.ReplyTo)
	}
	header.Set("Subject", mime.QEncoding.Encode("UTF-8", msg.Subject))
	header.Set("Date", time.Now().Format(time.RFC1123Z))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", contentType)
	for k, v := range msg.Headers {
		header.Set(k, v)
	}

	var out bytes.Buffer
	for _, k := range sortedHeaderKeys(header) {
		for _, v := range header[k] {
			fmt.Fprintf(&out, "%s: %s\r\n", k, v)
		}
	}
	out.WriteString("\r\n")
	out.Write(body)

	return out.Bytes(), nil
}

// buildBody picks msg's body encoding: a multipart/alternative envelope
// when both HTML and Text are set, otherwise whichever single one is.
func buildBody(msg *Message) ([]byte, string, error) {
	switch {
	case msg.HTML != "" && msg.Text != "":
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)

		textPart, err := mw.CreatePart(partHeader("text/plain; charset=UTF-8"))
		if err != nil {
			return nil, "", fmt.Errorf("mail: creating text part: %w", err)
		}
		if _, err := textPart.Write([]byte(msg.Text)); err != nil {
			return nil, "", fmt.Errorf("mail: writing text part: %w", err)
		}

		htmlPart, err := mw.CreatePart(partHeader("text/html; charset=UTF-8"))
		if err != nil {
			return nil, "", fmt.Errorf("mail: creating html part: %w", err)
		}
		if _, err := htmlPart.Write([]byte(msg.HTML)); err != nil {
			return nil, "", fmt.Errorf("mail: writing html part: %w", err)
		}

		if err := mw.Close(); err != nil {
			return nil, "", fmt.Errorf("mail: closing multipart writer: %w", err)
		}

		return body.Bytes(), fmt.Sprintf("multipart/alternative; boundary=%s", mw.Boundary()), nil

	case msg.HTML != "":
		return []byte(msg.HTML), "text/html; charset=UTF-8", nil

	default:
		return []byte(msg.Text), "text/plain; charset=UTF-8", nil
	}
}

func partHeader(contentType string) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Type", contentType)
	return h
}

func sortedHeaderKeys(h textproto.MIMEHeader) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
