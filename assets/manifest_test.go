package assets

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func newManifestFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}

	return fsys
}

func TestManifestPathReturnsDigestedPathWhenFound(t *testing.T) {
	m := Manifest{"app.css": "app-abc123.css"}

	digested, ok := m.Path("app.css")
	require.True(t, ok)
	require.Equal(t, "app-abc123.css", digested)
}

func TestManifestPathReturnsFalseWhenNotFound(t *testing.T) {
	m := Manifest{}

	digested, ok := m.Path("missing.css")
	require.False(t, ok)
	require.Equal(t, "", digested)
}

func TestLoadManifestReturnsParsedManifest(t *testing.T) {
	fsys := newManifestFS(map[string]string{
		"manifest.json": `{"app.css":"app-abc123.css","app.js":"app-def456.js"}`,
	})

	m, err := LoadManifest(fsys, "manifest.json")
	require.NoError(t, err)
	require.Equal(t, Manifest{"app.css": "app-abc123.css", "app.js": "app-def456.js"}, m)
}

func TestLoadManifestReturnsWrappedErrorWhenFileMissing(t *testing.T) {
	fsys := newManifestFS(map[string]string{})

	_, err := LoadManifest(fsys, "manifest.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest.json")
}

func TestLoadManifestReturnsWrappedErrorOnMalformedJSON(t *testing.T) {
	fsys := newManifestFS(map[string]string{
		"manifest.json": `{not valid json`,
	})

	_, err := LoadManifest(fsys, "manifest.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing manifest")
	require.Contains(t, err.Error(), "manifest.json")
}

func TestLoadManifestErrorsWhenJSONIsNotAnObject(t *testing.T) {
	fsys := newManifestFS(map[string]string{
		"manifest.json": `["a","b"]`,
	})

	_, err := LoadManifest(fsys, "manifest.json")
	require.Error(t, err)
}
