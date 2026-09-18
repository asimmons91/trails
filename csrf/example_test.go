package csrf_test

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/csrf"
	"github.com/asimmons91/trails/session"
)

// Example demonstrates the full wiring: session.Middleware and
// csrf.Middleware protect writes, while csrf.RequestFuncMap supplies the
// per-request token a form needs in order to be accepted.
func Example() {
	build := func(r *trails.Router) {
		r.Use(session.Middleware("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			field, ok := csrf.RequestFuncMap(c)["csrfField"].(func() (template.HTML, error))
			if !ok {
				return fmt.Errorf("csrfField helper missing")
			}
			html, err := field()
			if err != nil {
				return err
			}
			return c.HTML(http.StatusOK, string(html))
		})
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}

	trail, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		ViewFS:       fstest.MapFS{},
		AssetsFS:     fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:     fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		RouteBuilder: build,
	}))
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// A write with no token is rejected.
	unauth := httptest.NewRecorder()
	trail.ServeHTTP(unauth, httptest.NewRequest(http.MethodPost, "/write", nil))
	fmt.Println("unauthenticated:", unauth.Code)

	// Rendering the form supplies a token tied to the session cookie.
	formResp := httptest.NewRecorder()
	trail.ServeHTTP(formResp, httptest.NewRequest(http.MethodGet, "/form", nil))

	var cookie *http.Cookie
	for _, c := range formResp.Result().Cookies() {
		if c.Name == "_trails_session" {
			cookie = c
		}
	}

	const marker = `value="`
	body := formResp.Body.String()
	start := strings.Index(body, marker) + len(marker)
	token := body[start : strings.Index(body[start:], `"`)+start]

	// Submitting that token alongside its session cookie is accepted.
	form := url.Values{"authenticity_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/write", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	auth := httptest.NewRecorder()
	trail.ServeHTTP(auth, req)
	fmt.Println("authenticated:", auth.Code)

	// Output:
	// unauthenticated: 403
	// authenticated: 200
}
