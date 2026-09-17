package mail_test

import (
	"testing"
	"testing/fstest"

	"github.com/asimmons91/trails/mail"
	"github.com/stretchr/testify/require"
)

func testViewFS() fstest.MapFS {
	return fstest.MapFS{
		"layouts/mailer.gohtml": {Data: []byte(
			`{{ define "mailer" }}<html><body>{{ template "content" . }}{{ template "_footer" . }}</body></html>{{ end }}`,
		)},
		"layouts/_footer.gohtml": {Data: []byte(
			`{{ define "_footer" }}<footer>bye</footer>{{ end }}`,
		)},
		"user_mailer/welcome_email.gohtml": {Data: []byte(
			`{{ define "content" }}<p>Hi {{ .Name }}</p>{{ end }}`,
		)},
		"user_mailer/welcome_email.text.gohtml": {Data: []byte(
			`Hi {{ .Name }}`,
		)},
		"user_mailer/no_text.gohtml": {Data: []byte(
			`{{ define "content" }}<p>html only</p>{{ end }}`,
		)},
	}
}

func TestRenderReturnsHTMLWrappedInLayoutAndPartial(t *testing.T) {
	r, err := mail.NewRenderer(testViewFS(), "mailer", nil)
	require.NoError(t, err)

	html, _, err := r.Render("user_mailer/welcome_email", map[string]any{"Name": "Bob & Alice"})
	require.NoError(t, err)
	require.Equal(t, `<html><body><p>Hi Bob &amp; Alice</p><footer>bye</footer></body></html>`, html)
}

func TestRenderTextPartIsNotHTMLEscaped(t *testing.T) {
	r, err := mail.NewRenderer(testViewFS(), "mailer", nil)
	require.NoError(t, err)

	_, text, err := r.Render("user_mailer/welcome_email", map[string]any{"Name": "Bob & Alice"})
	require.NoError(t, err)
	require.Equal(t, "Hi Bob & Alice", text)
}

func TestRenderWithoutSiblingTextFileLeavesTextEmpty(t *testing.T) {
	r, err := mail.NewRenderer(testViewFS(), "mailer", nil)
	require.NoError(t, err)

	html, text, err := r.Render("user_mailer/no_text", nil)
	require.NoError(t, err)
	require.Contains(t, html, "html only")
	require.Empty(t, text)
}

func TestRenderUnknownTemplateErrors(t *testing.T) {
	r, err := mail.NewRenderer(testViewFS(), "mailer", nil)
	require.NoError(t, err)

	_, _, err = r.Render("user_mailer/missing", nil)
	require.Error(t, err)
}

func TestNewRendererErrorsWhenLayoutMissing(t *testing.T) {
	viewFS := testViewFS()
	delete(viewFS, "layouts/mailer.gohtml")

	_, err := mail.NewRenderer(viewFS, "mailer", nil)
	require.Error(t, err)
}
