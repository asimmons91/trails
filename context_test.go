package trails

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type errResponseWriter struct {
	header http.Header
}

func (w *errResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *errResponseWriter) WriteHeader(int) {}

func (w *errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type mockBinder struct {
	called bool
	ctx    *Context
	target any
}

func (m *mockBinder) Bind(c *Context, target any) error {
	m.called = true
	m.ctx = c
	m.target = target
	return nil
}

type mockRenderer struct {
	ctx     *Context
	name    string
	data    any
	written []byte
	err     error

	namedAction string
	namedBlock  string
	namedHTML   template.HTML
	namedErr    error
}

func (m *mockRenderer) Render(c *Context, w io.Writer, name string, data any) error {
	m.ctx = c
	m.name = name
	m.data = data
	if m.err != nil {
		return m.err
	}
	_, err := w.Write(m.written)
	return err
}

func (m *mockRenderer) RenderNamed(action, block string, data any) (template.HTML, error) {
	m.namedAction = action
	m.namedBlock = block
	m.data = data
	if m.namedErr != nil {
		return "", m.namedErr
	}
	return m.namedHTML, nil
}

func TestContextGetSetRoundTrip(t *testing.T) {
	c := newContext(nil, nil, nil)
	c.Set("key", "value")
	require.Equal(t, "value", c.Get[string]("key"))
}

func TestContextHTMLBlob(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	err := c.HTMLBlob(200, []byte("<p>hi</p>"))
	require.NoError(t, err)
	require.Equal(t, "<p>hi</p>", w.Body.String())
	require.Equal(t, "text/html; charset=UTF-8", w.Header().Get("Content-Type"))
}

func TestContextRenderWritesTemplateOutput(t *testing.T) {
	renderer := &mockRenderer{written: []byte("<p>hi</p>")}
	trail := &Trail{renderer: renderer}
	w := httptest.NewRecorder()
	c := newContext(w, nil, trail)

	data := map[string]string{"a": "b"}
	err := c.Render(200, "index", data)
	require.NoError(t, err)

	require.Equal(t, 200, w.Code)
	require.Equal(t, "<p>hi</p>", w.Body.String())
	require.Equal(t, "text/html; charset=UTF-8", w.Header().Get("Content-Type"))

	require.Same(t, c, renderer.ctx)
	require.Equal(t, "index", renderer.name)
	require.Equal(t, data, renderer.data)
}

func TestContextRenderReturnsWrappedRendererError(t *testing.T) {
	sentinel := errors.New("boom")
	renderer := &mockRenderer{err: sentinel}
	trail := &Trail{renderer: renderer}
	w := httptest.NewRecorder()
	c := newContext(w, nil, trail)

	err := c.Render(200, "index", nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "index")
	require.ErrorIs(t, err, sentinel)

	require.Empty(t, w.Body.String())
	require.Empty(t, w.Header().Get("Content-Type"))
}

func TestContextRenderBlockUsesActionFromLastRender(t *testing.T) {
	renderer := &mockRenderer{written: []byte("<p>hi</p>"), namedHTML: template.HTML("<span>card</span>")}
	trail := &Trail{renderer: renderer}
	w := httptest.NewRecorder()
	c := newContext(w, nil, trail)

	require.NoError(t, c.Render(200, "products/show", nil))

	html, err := c.RenderBlock("card", map[string]string{"id": "1"})
	require.NoError(t, err)
	require.Equal(t, template.HTML("<span>card</span>"), html)
	require.Equal(t, "products/show", renderer.namedAction)
	require.Equal(t, "card", renderer.namedBlock)
}

func TestContextRenderBlockBeforeRenderReturnsError(t *testing.T) {
	renderer := &mockRenderer{}
	trail := &Trail{renderer: renderer}
	c := newContext(nil, nil, trail)

	_, err := c.RenderBlock("card", nil)
	require.Error(t, err)
}

func TestContextRenderBlockWrapsRendererError(t *testing.T) {
	sentinel := errors.New("boom")
	renderer := &mockRenderer{written: []byte("<p>hi</p>"), namedErr: sentinel}
	trail := &Trail{renderer: renderer}
	w := httptest.NewRecorder()
	c := newContext(w, nil, trail)
	require.NoError(t, c.Render(200, "products/show", nil))

	_, err := c.RenderBlock("card", nil)
	require.ErrorIs(t, err, sentinel)
}

func TestContextSetResponseSwapsWriter(t *testing.T) {
	c := newContext(httptest.NewRecorder(), nil, nil)

	replacement := httptest.NewRecorder()
	c.SetResponse(replacement)

	require.Same(t, http.ResponseWriter(replacement), c.Response())
}

func TestContextGetMissingKeyReturnsZeroValue(t *testing.T) {
	c := newContext(nil, nil, nil)
	require.Equal(t, "", c.Get[string]("missing"))
	require.Equal(t, 0, c.Get[int]("missing"))
}

func TestContextGetTypeMismatchReturnsZeroValue(t *testing.T) {
	c := newContext(nil, nil, nil)
	c.Set("key", "value")
	require.Equal(t, 0, c.Get[int]("key"))
}

func TestContextReset(t *testing.T) {
	c := newContext(nil, nil, nil)
	c.Set("key", "value")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	c.Reset(w, r)

	require.Equal(t, "", c.Get[string]("key"))
	require.Same(t, w, c.Response())
	require.Same(t, r, c.Request())
}

func TestContextRequestResponseAccessors(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	c := newContext(w, r, nil)

	require.Same(t, w, c.Response())
	require.Same(t, r, c.Request())
}

func TestContextLoggerAccessor(t *testing.T) {
	logger := slog.Default()
	trail := &Trail{logger: logger}
	c := newContext(nil, nil, trail)

	require.Same(t, logger, c.Logger())
}

type testContextKey string

func TestContextContextAccessor(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey("key"), "value")
	trail := &Trail{context: ctx}
	c := newContext(nil, nil, trail)

	require.Equal(t, ctx, c.Context())
}

func TestContextBindDelegatesToBinder(t *testing.T) {
	binder := &mockBinder{}
	trail := &Trail{binder: binder}
	c := newContext(nil, nil, trail)

	target := &struct{}{}
	err := c.Bind(target)
	require.NoError(t, err)
	require.True(t, binder.called)
	require.Same(t, c, binder.ctx)
	require.Same(t, target, binder.target)
}

func TestContextBlobReturnsWriteError(t *testing.T) {
	w := &errResponseWriter{}
	c := newContext(w, nil, nil)

	err := c.Blob(200, "text/plain", []byte("payload"))
	require.Error(t, err)
}

func TestContextBlobWritesStatusBodyAndContentType(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	err := c.Blob(201, "application/custom", []byte("payload"))
	require.NoError(t, err)
	require.Equal(t, 201, w.Code)
	require.Equal(t, "payload", w.Body.String())
	require.Equal(t, "application/custom", w.Header().Get("Content-Type"))
}

func TestContextBlobDoesNotOverrideExistingContentType(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "text/existing")
	c := newContext(w, nil, nil)

	err := c.Blob(200, "application/custom", []byte("payload"))
	require.NoError(t, err)
	require.Equal(t, "text/existing", w.Header().Get("Content-Type"))
}

func TestContextString(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	err := c.String(200, "hello")
	require.NoError(t, err)
	require.Equal(t, "hello", w.Body.String())
	require.Equal(t, "text/plain; charset=UTF-8", w.Header().Get("Content-Type"))
}

func TestContextHTML(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	err := c.HTML(200, "<p>hi</p>")
	require.NoError(t, err)
	require.Equal(t, "<p>hi</p>", w.Body.String())
	require.Equal(t, "text/html; charset=UTF-8", w.Header().Get("Content-Type"))
}

func TestContextJSON(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	payload := map[string]string{"hello": "world"}
	err := c.JSON(200, payload)
	require.NoError(t, err)

	expected := &bytes.Buffer{}
	require.NoError(t, json.NewEncoder(expected).Encode(payload))

	require.Equal(t, expected.String(), w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestContextJSONDoesNotOverrideExistingContentType(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "text/existing")
	c := newContext(w, nil, nil)

	err := c.JSON(200, map[string]string{"a": "b"})
	require.NoError(t, err)
	require.Equal(t, "text/existing", w.Header().Get("Content-Type"))
}

func TestContextJSONReturnsEncodeError(t *testing.T) {
	w := httptest.NewRecorder()
	c := newContext(w, nil, nil)

	err := c.JSON(200, make(chan int))
	require.Error(t, err)
}

func TestContextJSONReturnsWriteError(t *testing.T) {
	w := &errResponseWriter{}
	c := newContext(w, nil, nil)

	err := c.JSON(200, map[string]string{"a": "b"})
	require.Error(t, err)
}

func TestStringToBytesEmptyString(t *testing.T) {
	require.Nil(t, stringToBytes(""))
}

func TestStringToBytesNonEmptyString(t *testing.T) {
	require.Equal(t, []byte("hello"), stringToBytes("hello"))
}

func TestContextConcurrentGetSet(t *testing.T) {
	c := newContext(nil, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Set("key", "value")
		}()
		go func() {
			defer wg.Done()
			_ = c.Get[string]("key")
		}()
	}
	wg.Wait()
}
