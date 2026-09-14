package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSourceTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for relPath, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(relPath))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	return dir
}

func TestCompileHappyPathMultipleFiles(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css":    "body{color:red;}",
		"app.js":     "console.log(1);",
		"app.js.map": `{"version":3}`,
	})
	out := t.TempDir()

	manifest, err := Compile(src, out, "/static")
	require.NoError(t, err)
	require.Len(t, manifest, 3)

	require.Regexp(t, `^app-[0-9a-f]{16}\.css$`, manifest["app.css"])
	require.Regexp(t, `^app-[0-9a-f]{16}\.js$`, manifest["app.js"])
	require.Regexp(t, `^app\.js-[0-9a-f]{16}\.map$`, manifest["app.js.map"])

	for logical, digested := range manifest {
		content, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(digested)))
		require.NoError(t, err, "reading output for %s", logical)
		require.NotEmpty(t, content)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	require.NoError(t, err)
	var onDisk Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &onDisk))
	require.Equal(t, manifest, onDisk)
}

func TestCompileRewritesCSSImportURLToDigestedPath(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css": `@import url("lib.css");` + "\nbody{color:red;}",
		"lib.css": ".lib{color:blue;}",
	})
	out := t.TempDir()

	manifest, err := Compile(src, out, "/static")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(manifest["app.css"])))
	require.NoError(t, err)
	require.Contains(t, string(content), `@import url("/static/`+manifest["lib.css"]+`")`)
	require.NotContains(t, string(content), `"lib.css"`)
}

func TestCompileRewritesJSSourceMappingURLToDigestedPath(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.js":     "console.log(1);\n//# sourceMappingURL=app.js.map",
		"app.js.map": `{"version":3}`,
	})
	out := t.TempDir()

	manifest, err := Compile(src, out, "/static")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(manifest["app.js"])))
	require.NoError(t, err)
	require.Contains(t, string(content), "//# sourceMappingURL=/static/"+manifest["app.js.map"])
}

func TestCompileSkipsExternalReferencesUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"http", "http://cdn.example.com/foo.css"},
		{"https", "https://cdn.example.com/foo.css"},
		{"protocol relative", "//cdn.example.com/foo.css"},
		{"data uri", "data:text/css;base64,Zm9v"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			importLine := `@import url("` + tc.target + `");`
			src := writeSourceTree(t, map[string]string{
				"app.css": importLine,
			})
			out := t.TempDir()

			manifest, err := Compile(src, out, "/static")
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(manifest["app.css"])))
			require.NoError(t, err)
			require.Equal(t, importLine, string(content))
		})
	}
}

func TestCompileErrorsWhenReferencedLocalFileDoesNotExist(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css": `@import url("missing.css");`,
	})
	out := t.TempDir()

	_, err := Compile(src, out, "/static")
	require.Error(t, err)
	require.Contains(t, err.Error(), "app.css")
	require.Contains(t, err.Error(), "missing.css")
}

func TestCompileDigestChangesWhenReferencedFileContentChanges(t *testing.T) {
	appCSS := `@import url("lib.css");`

	src1 := writeSourceTree(t, map[string]string{
		"app.css": appCSS,
		"lib.css": ".lib{color:red;}",
	})
	manifest1, err := Compile(src1, t.TempDir(), "/static")
	require.NoError(t, err)

	src2 := writeSourceTree(t, map[string]string{
		"app.css": appCSS,
		"lib.css": ".lib{color:blue;}",
	})
	manifest2, err := Compile(src2, t.TempDir(), "/static")
	require.NoError(t, err)

	require.NotEqual(t, manifest1["app.css"], manifest2["app.css"])
}

func TestCompileIgnoresNonAssetFileExtensions(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css":    "body{}",
		"readme.txt": "hello",
		"image.png":  "not-really-a-png",
	})
	out := t.TempDir()

	manifest, err := Compile(src, out, "/static")
	require.NoError(t, err)
	require.Len(t, manifest, 1)
	require.Contains(t, manifest, "app.css")

	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), "readme")
		require.NotContains(t, e.Name(), "image")
	}
}

func TestCompileHandlesNestedDirectories(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"styles/app.css":        `@import url("vendor/lib.css");`,
		"styles/vendor/lib.css": ".lib{}",
	})
	out := t.TempDir()

	manifest, err := Compile(src, out, "/static")
	require.NoError(t, err)

	digested, ok := manifest["styles/app.css"]
	require.True(t, ok)
	require.True(t, strings.HasPrefix(digested, "styles/app-"))

	_, err = os.Stat(filepath.Join(out, filepath.FromSlash(digested)))
	require.NoError(t, err)
}

func TestCompileCleansOutputDirBeforeWriting(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css": "body{}",
	})
	out := t.TempDir()
	stale := filepath.Join(out, "stale.txt")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o644))

	_, err := Compile(src, out, "/static")
	require.NoError(t, err)

	_, err = os.Stat(stale)
	require.True(t, os.IsNotExist(err))
}

func TestCompileReturnsErrorWhenSourceDirMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	out := t.TempDir()

	_, err := Compile(missing, out, "/static")
	require.Error(t, err)
	require.Contains(t, err.Error(), "walking")
}

func TestReadSourceFilesFiltersByExtension(t *testing.T) {
	src := writeSourceTree(t, map[string]string{
		"app.css":          "a",
		"app.js":           "b",
		"app.js.map":       "c",
		"readme.txt":       "d",
		"image.png":        "e",
		"nested/vendor.js": "f",
	})

	files, err := readSourceFiles(src)
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{
		"app.css":          []byte("a"),
		"app.js":           []byte("b"),
		"app.js.map":       []byte("c"),
		"nested/vendor.js": []byte("f"),
	}, files)
}

func TestDigestedName(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		digest  string
		want    string
	}{
		{"simple file", "app.css", "abc123", "app-abc123.css"},
		{"nested file", "styles/app.css", "abc123", "styles/app-abc123.css"},
		{"extensionless file", "app", "abc123", "app-abc123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, digestedName(tc.relPath, tc.digest))
		})
	}
}

func TestResolveImportTargetCleansRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		target  string
		want    string
	}{
		{"sibling file", "app.css", "lib.css", "lib.css"},
		{"parent traversal", "styles/app.css", "../shared/reset.css", "shared/reset.css"},
		{"current dir prefix", "a/b/app.css", "./x.css", "a/b/x.css"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, resolveImportTarget(tc.relPath, tc.target))
		})
	}
}

func TestIsExternal(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"http", "http://cdn.example.com/foo.css", true},
		{"https", "https://cdn.example.com/foo.css", true},
		{"protocol relative", "//cdn.example.com/foo.css", true},
		{"data uri", "data:text/css;base64,Zm9v", true},
		{"relative path", "lib.css", false},
		{"dot relative path", "./lib.css", false},
		{"absolute local path", "/abs/lib.css", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isExternal(tc.target))
		})
	}
}

func TestDigestIncludesOwnAndReferencedContent(t *testing.T) {
	files := map[string][]byte{
		"app.css": []byte("body{}"),
		"lib.css": []byte(".lib{}"),
	}
	refs := map[string][]string{
		"app.css": {"lib.css"},
	}

	got := digest(files, refs, "app.css")

	h := sha256.New()
	h.Write(files["app.css"])
	h.Write(files["lib.css"])
	want := hex.EncodeToString(h.Sum(nil))[:16]

	require.Equal(t, want, got)
}

func TestCompileDigestAccountsForTransitiveReferences(t *testing.T) {
	aCSS := `@import url("b.css");`
	bCSS := `@import url("c.css");`

	src1 := writeSourceTree(t, map[string]string{
		"a.css": aCSS,
		"b.css": bCSS,
		"c.css": ".c{color:red;}",
	})
	out1 := t.TempDir()
	manifest1, err := Compile(src1, out1, "/static")
	require.NoError(t, err)
	aOut1, err := os.ReadFile(filepath.Join(out1, filepath.FromSlash(manifest1["a.css"])))
	require.NoError(t, err)

	src2 := writeSourceTree(t, map[string]string{
		"a.css": aCSS,
		"b.css": bCSS,
		"c.css": ".c{color:blue;}",
	})
	out2 := t.TempDir()
	manifest2, err := Compile(src2, out2, "/static")
	require.NoError(t, err)
	aOut2, err := os.ReadFile(filepath.Join(out2, filepath.FromSlash(manifest2["a.css"])))
	require.NoError(t, err)

	require.NotEqual(t, manifest1["b.css"], manifest2["b.css"],
		"b.css digest should change: it directly references c.css")
	require.NotEqual(t, manifest1["a.css"], manifest2["a.css"],
		"a.css digest should change: its transitive dependency (c.css) changed")
	require.NotEqual(t, string(aOut1), string(aOut2),
		"a.css output content differs (rewritten import points at b.css's new digested name)")
}

func TestDigestIncludesTransitivelyReferencedContent(t *testing.T) {
	files := map[string][]byte{
		"a.css": []byte("@import a"),
		"b.css": []byte("@import b"),
		"c.css": []byte("original"),
	}
	refs := map[string][]string{
		"a.css": {"b.css"},
		"b.css": {"c.css"},
	}

	before := digest(files, refs, "a.css")

	files["c.css"] = []byte("changed")
	after := digest(files, refs, "a.css")

	require.NotEqual(t, before, after,
		"a.css digest should depend on c.css's content even though c.css is only referenced transitively (via b.css)")
}

func TestDigestHandlesCyclicReferencesWithoutHanging(t *testing.T) {
	files := map[string][]byte{
		"a.css": []byte("a"),
		"b.css": []byte("b"),
	}
	refs := map[string][]string{
		"a.css": {"b.css"},
		"b.css": {"a.css"},
	}

	got := digest(files, refs, "a.css")
	require.Len(t, got, 16)
}
