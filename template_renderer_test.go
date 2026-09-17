package trails

import (
	"bytes"
	"errors"
	"html/template"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

type erroringFS struct {
	fs.FS
	failReadDir string
}

func (e erroringFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == e.failReadDir {
		return nil, errors.New("boom")
	}
	return fs.ReadDir(e.FS, name)
}

func mapFile(data string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(data)}
}

func TestNewTemplateRendererBuildsActionKeyedByControllerAndName(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}<html>{{template "content" .}}</html>{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}Hello, {{.Name}}!{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = renderer.Render(nil, &buf, "posts/index", map[string]string{"Name": "World"})
	require.NoError(t, err)
	require.Equal(t, "<html>Hello, World!</html>", buf.String())
}

func TestNewTemplateRendererMakesPartialsAvailableButNotRegistered(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/_nav.gohtml":          mapFile(`{{define "nav"}}[nav]{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}{{template "nav" .}}body{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	_, ok := tr.templates["posts/_nav"]
	require.False(t, ok)
	_, ok = tr.templates["posts/nav"]
	require.False(t, ok)

	var buf bytes.Buffer
	err = renderer.Render(nil, &buf, "posts/index", nil)
	require.NoError(t, err)
	require.Equal(t, "[nav]body", buf.String())
}

func TestRenderNamedRendersBlockFromActionTemplateSet(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/_card.gohtml":         mapFile(`{{define "card"}}card:{{.Name}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}{{template "card" .}}{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	html, err := renderer.RenderNamed("posts/index", "card", map[string]string{"Name": "World"})
	require.NoError(t, err)
	require.Equal(t, template.HTML("card:World"), html)
}

func TestRenderNamedReturnsErrorForUnknownAction(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	_, err = renderer.RenderNamed("posts/missing", "card", nil)
	require.Error(t, err)
}

func TestRenderNamedReturnsErrorForUnknownBlock(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	_, err = renderer.RenderNamed("posts/index", "missing", nil)
	require.Error(t, err)
}

func TestNewTemplateRendererSkipsLayoutsDirectoryAsController(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	for key := range tr.templates {
		require.False(t, strings.HasPrefix(key, "layouts/"), "unexpected layouts key %q", key)
	}
}

func TestNewTemplateRendererSkipsNonGohtmlFilesInControllerDir(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
		"posts/notes.txt":            mapFile("not a template"),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	require.Len(t, tr.templates, 1)
	_, ok := tr.templates["posts/index"]
	require.True(t, ok)
}

func TestNewTemplateRendererSkipsNonDirectoryEntriesAtRoot(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
		"README.md":                  mapFile("not a controller"),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	require.Len(t, tr.templates, 1)
}

func TestNewTemplateRendererSupportsMultipleControllersAndActions(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}posts-index{{end}}`),
		"posts/show.gohtml":          mapFile(`{{define "content"}}posts-show{{end}}`),
		"users/index.gohtml":         mapFile(`{{define "content"}}users-index{{end}}`),
		"users/show.gohtml":          mapFile(`{{define "content"}}users-show{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	require.Len(t, tr.templates, 4)
	for _, key := range []string{"posts/index", "posts/show", "users/index", "users/show"} {
		_, ok := tr.templates[key]
		require.True(t, ok, "expected key %q", key)
	}
}

func TestNewTemplateRendererWiresFuncMapIntoTemplates(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}{{shout .Name}}{{end}}`),
	}
	funcs := template.FuncMap{
		"shout": func(s string) string { return strings.ToUpper(s) + "!" },
	}

	renderer, err := newTemplateRenderer(viewFS, "application", funcs)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = renderer.Render(nil, &buf, "posts/index", map[string]string{"Name": "world"})
	require.NoError(t, err)
	require.Equal(t, "WORLD!", buf.String())
}

func TestNewTemplateRendererReturnsErrorWhenViewsRootUnreadable(t *testing.T) {
	viewFS := erroringFS{
		FS:          fstest.MapFS{},
		failReadDir: ".",
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.Nil(t, renderer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read views root")
}

func TestNewTemplateRendererReturnsErrorWhenControllerDirUnreadable(t *testing.T) {
	viewFS := erroringFS{
		FS: fstest.MapFS{
			"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
			"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
		},
		failReadDir: "posts",
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.Nil(t, renderer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read posts")
}

func TestNewTemplateRendererReturnsErrorOnParseFailure(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}{{.Broken`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.Nil(t, renderer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "renderer: parse")
	require.Contains(t, err.Error(), "posts/index")
}

func TestNewTemplateRendererLoadsUnderscoredLayoutPartial(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "flash" .}}{{template "content" .}}{{end}}`),
		"layouts/_flash.gohtml":      mapFile(`{{define "flash"}}[flash]{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}body{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = renderer.Render(nil, &buf, "posts/index", nil)
	require.NoError(t, err)
	require.Equal(t, "[flash]body", buf.String())
}

func TestNewTemplateRendererExcludesNonMatchingLayoutFile(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/application.gohtml": mapFile(`{{define "application"}}{{template "content" .}}{{end}}`),
		"layouts/admin.gohtml":       mapFile(`{{define "adminOnlyMarker"}}unused{{end}}`),
		"posts/index.gohtml":         mapFile(`{{define "content"}}hi{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)

	tr := renderer.(*templateRenderer)
	tmpl := tr.templates["posts/index"]
	require.NotNil(t, tmpl.Lookup("application"))
	require.Nil(t, tmpl.Lookup("adminOnlyMarker"))
}

func TestNewTemplateRendererSucceedsWithNoLayoutsDirectory(t *testing.T) {
	viewFS := fstest.MapFS{}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)

	tr := renderer.(*templateRenderer)
	require.Len(t, tr.templates, 0)
}

func TestNewTemplateRendererReturnsErrorWhenNoLayoutMatchesConfiguredName(t *testing.T) {
	viewFS := fstest.MapFS{
		"layouts/other.gohtml": mapFile(`{{define "other"}}{{template "content" .}}{{end}}`),
		"posts/index.gohtml":   mapFile(`{{define "content"}}hi{{end}}`),
	}

	renderer, err := newTemplateRenderer(viewFS, "application", nil)
	require.Nil(t, renderer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "renderer: layout")
	require.Contains(t, err.Error(), `"application"`)
}

func TestRenderReturnsErrorForUnknownTemplateName(t *testing.T) {
	tr := &templateRenderer{templates: map[string]*template.Template{}}

	var buf bytes.Buffer
	err := tr.Render(nil, &buf, "missing/name", nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), `template "missing/name" not found`)
	require.Equal(t, "", buf.String())
}

func TestRenderExecutesNamedLayoutTemplateWithData(t *testing.T) {
	tmpl := template.Must(template.New("index").Parse(`{{define "application"}}hello {{.}}{{end}}`))
	tr := &templateRenderer{
		templates:  map[string]*template.Template{"posts/index": tmpl},
		layoutName: "application",
	}

	var buf bytes.Buffer
	err := tr.Render(nil, &buf, "posts/index", "world")

	require.NoError(t, err)
	require.Equal(t, "hello world", buf.String())
}

func TestRenderPropagatesExecuteTemplateErrorWhenLayoutMissing(t *testing.T) {
	tmpl := template.Must(template.New("index").Parse(`{{define "content"}}hi{{end}}`))
	tr := &templateRenderer{
		templates:  map[string]*template.Template{"posts/index": tmpl},
		layoutName: "application",
	}

	var buf bytes.Buffer
	err := tr.Render(nil, &buf, "posts/index", nil)

	require.Error(t, err)
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRenderPropagatesWriterError(t *testing.T) {
	tmpl := template.Must(template.New("index").Parse(`{{define "application"}}hello{{end}}`))
	tr := &templateRenderer{
		templates:  map[string]*template.Template{"posts/index": tmpl},
		layoutName: "application",
	}

	err := tr.Render(nil, errWriter{}, "posts/index", nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "write failed")
}
