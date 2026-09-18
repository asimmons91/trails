package mail_test

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/asimmons91/trails/mail"
	"github.com/asimmons91/trails/mail/mailtest"
)

// Example builds a Renderer from a view directory (a layout, a shared
// partial, and a mailer action with both an HTML and a sibling text
// template), then delivers through it. It shows both NewRenderer's
// directory convention and Deliver's default-From behavior: the Message
// has no From set, so it's filled in from the Mailer's default.
func Example() {
	viewFS := fstest.MapFS{
		"layouts/mailer.gohtml": {Data: []byte(
			`{{ define "mailer" }}<html><body>{{ template "content" . }}{{ template "_footer" . }}</body></html>{{ end }}`,
		)},
		"layouts/_footer.gohtml": {Data: []byte(
			`{{ define "_footer" }}<footer>Sent by Trails</footer>{{ end }}`,
		)},
		"user_mailer/welcome_email.gohtml": {Data: []byte(
			`{{ define "content" }}<p>Hi {{ .Name }}</p>{{ end }}`,
		)},
		"user_mailer/welcome_email.text.gohtml": {Data: []byte(
			`Hi {{ .Name }}`,
		)},
	}

	renderer, err := mail.NewRenderer(viewFS, "mailer", nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	recorder := mailtest.NewRecorder()
	mailer := mail.NewMailer(renderer, recorder, "noreply@example.com")

	err = mailer.Deliver(context.Background(), "user_mailer/welcome_email", mail.Message{
		To:      []string{"user@example.com"},
		Subject: "Welcome!",
	}, map[string]any{"Name": "Ada"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	sent := recorder.Sent()[0]
	fmt.Println("from:", sent.From)
	fmt.Println("html:", sent.HTML)
	fmt.Println("text:", sent.Text)
	// Output:
	// from: noreply@example.com
	// html: <html><body><p>Hi Ada</p><footer>Sent by Trails</footer></body></html>
	// text: Hi Ada
}
