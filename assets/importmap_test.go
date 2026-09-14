package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newImportMapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}

	return fsys
}

// mockHTTPDoer implements httpDoer via testify/mock, so JspmResolver.Generate
// (which posts to a hardcoded external URL) can be tested without a real
// network call.
type mockHTTPDoer struct {
	mock.Mock
}

func (m *mockHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	resp, _ := args.Get(0).(*http.Response)

	return resp, args.Error(1)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestPinResolve(t *testing.T) {
	tests := []struct {
		name       string
		pin        Pin
		manifest   Manifest
		prefix     string
		want       ResolvedEntry
		wantErr    bool
		wantErrSub string
	}{
		{
			name:     "default lookup success with preload",
			pin:      Pin{Name: "application", Preload: true},
			manifest: Manifest{"application.js": "application-abc.js"},
			prefix:   "/static",
			want:     ResolvedEntry{Name: "application", URL: "/static/application-abc.js", Preload: true},
		},
		{
			name:       "default lookup missing manifest entry",
			pin:        Pin{Name: "missing"},
			manifest:   Manifest{},
			prefix:     "/static",
			wantErr:    true,
			wantErrSub: `no manifest entry for "missing.js"`,
		},
		{
			name:     "external to returns url verbatim without prefix",
			pin:      Pin{Name: "foo", To: "https://cdn.example.com/foo.js"},
			manifest: Manifest{},
			prefix:   "/static",
			want:     ResolvedEntry{Name: "foo", URL: "https://cdn.example.com/foo.js"},
		},
		{
			name:     "vendor to lookup success",
			pin:      Pin{Name: "foo", To: "foo.js"},
			manifest: Manifest{"vendor/foo.js": "vendor/foo-abc.js"},
			prefix:   "/static",
			want:     ResolvedEntry{Name: "foo", URL: "/static/vendor/foo-abc.js"},
		},
		{
			name:       "vendor to lookup missing manifest entry",
			pin:        Pin{Name: "foo", To: "foo.js"},
			manifest:   Manifest{},
			prefix:     "/static",
			wantErr:    true,
			wantErrSub: `no manifest entry for "vendor/foo.js"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.pin.resolve(tc.manifest, tc.prefix)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrSub != "" {
					require.Contains(t, err.Error(), tc.wantErrSub)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPinAllResolveDefaultPatternMatchesAllJS(t *testing.T) {
	m := Manifest{
		"vendor/foo.js":    "vendor/foo-abc.js",
		"vendor/bar.js":    "vendor/bar-def.js",
		"vendor/style.css": "vendor/style-ghi.css",
	}
	pa := PinAll{Dir: "vendor", Under: "vendor"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "vendor/bar", entries[0].Name)
	require.Equal(t, "vendor/foo", entries[1].Name)
}

func TestPinAllResolveSortsEntriesByName(t *testing.T) {
	m := Manifest{
		"vendor/zeta.js":  "vendor/zeta-abc.js",
		"vendor/alpha.js": "vendor/alpha-def.js",
		"vendor/mid.js":   "vendor/mid-ghi.js",
	}
	pa := PinAll{Dir: "vendor"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 3)
	require.Equal(t, []string{"alpha", "mid", "zeta"},
		[]string{entries[0].Name, entries[1].Name, entries[2].Name})
}

func TestPinAllResolveAppliesUnderPrefix(t *testing.T) {
	m := Manifest{"vendor/libs/foo.js": "vendor/libs/foo-abc.js"}
	pa := PinAll{Dir: "vendor/libs", Under: "external"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "external/foo", entries[0].Name)
}

func TestPinAllResolveCustomPattern(t *testing.T) {
	m := Manifest{
		"vendor/foo.js":  "vendor/foo-abc.js",
		"vendor/foo.mjs": "vendor/foo-def.mjs",
	}
	pa := PinAll{Dir: "vendor", Pattern: "*.mjs"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "foo", entries[0].Name)
	require.Equal(t, "/static/vendor/foo-def.mjs", entries[0].URL)
}

func TestPinAllResolveReturnsErrorOnBadPattern(t *testing.T) {
	m := Manifest{"vendor/foo.js": "vendor/foo-abc.js"}
	pa := PinAll{Dir: "vendor", Pattern: "[unterminated"}

	_, err := pa.resolve(m, "/static")
	require.Error(t, err)
	require.Contains(t, err.Error(), "vendor")
	require.Contains(t, err.Error(), "bad pattern")
}

func TestPinAllResolveReturnsEmptySliceWhenNoMatches(t *testing.T) {
	m := Manifest{"other/foo.js": "other/foo-abc.js"}
	pa := PinAll{Dir: "vendor"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPinAllResolveCustomPatternMatchesSubdirectoryPath(t *testing.T) {
	m := Manifest{"vendor/sub/foo.js": "vendor/sub/foo-abc.js"}
	pa := PinAll{Dir: "vendor", Pattern: "sub/*.js"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "sub/foo", entries[0].Name)
}

func TestPinAllResolveNonRecursivePatternDoesNotMatchNestedFiles(t *testing.T) {
	m := Manifest{
		"vendor/foo.js":     "vendor/foo-abc.js",
		"vendor/sub/foo.js": "vendor/sub/foo-def.js",
	}
	pa := PinAll{Dir: "vendor", Pattern: "*.js"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "foo", entries[0].Name)
}

func TestPinAllResolveDefaultPatternMatchesNestedFiles(t *testing.T) {
	m := Manifest{"vendor/sub/foo.js": "vendor/sub/foo-abc.js"}
	pa := PinAll{Dir: "vendor"}

	entries, err := pa.resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "sub/foo", entries[0].Name)
}

func TestImportMapSaveWritesTOMLFile(t *testing.T) {
	im := ImportMap{Pins: []Pin{{Name: "application", Preload: true}}}
	path := filepath.Join(t.TempDir(), "importmap.toml")

	err := im.Save(path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var roundTrip ImportMap
	require.NoError(t, toml.Unmarshal(data, &roundTrip))
	require.Equal(t, im.Pins, roundTrip.Pins)
	require.Empty(t, roundTrip.PinAlls)
}

func TestImportMapSaveReturnsWrappedErrorOnUnwritablePath(t *testing.T) {
	im := ImportMap{}
	path := filepath.Join(t.TempDir(), "nonexistent-subdir", "x.toml")

	err := im.Save(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "writing importmap")
}

func TestImportMapAddPinAppendsNewPin(t *testing.T) {
	im := ImportMap{}
	im.AddPin(Pin{Name: "a"})
	require.Equal(t, []Pin{{Name: "a"}}, im.Pins)
}

func TestImportMapAddPinReplacesExistingByName(t *testing.T) {
	im := ImportMap{Pins: []Pin{{Name: "a", To: "old.js"}}}
	im.AddPin(Pin{Name: "a", To: "new.js"})
	require.Equal(t, []Pin{{Name: "a", To: "new.js"}}, im.Pins)
}

func TestImportMapRemovePinRemovesFoundPin(t *testing.T) {
	im := ImportMap{Pins: []Pin{{Name: "a"}, {Name: "b"}}}

	removed, ok := im.RemovePin("a")
	require.True(t, ok)
	require.Equal(t, Pin{Name: "a"}, removed)
	require.Equal(t, []Pin{{Name: "b"}}, im.Pins)
}

func TestImportMapRemovePinReturnsFalseWhenNotFound(t *testing.T) {
	im := ImportMap{Pins: []Pin{{Name: "a"}}}

	removed, ok := im.RemovePin("missing")
	require.False(t, ok)
	require.Equal(t, Pin{}, removed)
	require.Equal(t, []Pin{{Name: "a"}}, im.Pins)
}

func TestImportMapResolveConcatenatesPinsAndPinAllsInOrder(t *testing.T) {
	im := ImportMap{
		Pins:    []Pin{{Name: "application"}},
		PinAlls: []PinAll{{Dir: "vendor"}},
	}
	m := Manifest{
		"application.js": "application-abc.js",
		"vendor/foo.js":  "vendor/foo-def.js",
	}

	entries, err := im.Resolve(m, "/static")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "application", entries[0].Name)
	require.Equal(t, "foo", entries[1].Name)
}

func TestImportMapResolvePropagatesPinError(t *testing.T) {
	im := ImportMap{
		Pins:    []Pin{{Name: "missing"}},
		PinAlls: []PinAll{{Dir: "vendor", Pattern: "["}},
	}
	m := Manifest{"vendor/foo.js": "vendor/foo-abc.js"}

	_, err := im.Resolve(m, "/static")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
	require.NotContains(t, err.Error(), "bad pattern")
}

func TestImportMapResolvePropagatesPinAllError(t *testing.T) {
	im := ImportMap{
		PinAlls: []PinAll{{Dir: "vendor", Pattern: "["}},
	}
	m := Manifest{"vendor/foo.js": "vendor/foo-abc.js"}

	_, err := im.Resolve(m, "/static")
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad pattern")
}

func TestLoadImportMapConfigParsesPinsAndPinAlls(t *testing.T) {
	src := `
[[pin]]
name = "application"
preload = true

[[pin_all]]
dir = "vendor"
under = "vendor"
preload = false
`
	fsys := newImportMapFS(map[string]string{"importmap.toml": src})

	im, err := LoadImportMapConfig(fsys, "importmap.toml")
	require.NoError(t, err)
	require.Equal(t, []Pin{{Name: "application", Preload: true}}, im.Pins)
	require.Equal(t, []PinAll{{Dir: "vendor", Under: "vendor", Preload: false}}, im.PinAlls)
}

func TestLoadImportMapConfigReturnsWrappedErrorWhenFileMissing(t *testing.T) {
	fsys := newImportMapFS(map[string]string{})

	_, err := LoadImportMapConfig(fsys, "importmap.toml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "importmap.toml")
}

func TestLoadImportMapConfigReturnsWrappedErrorOnMalformedTOML(t *testing.T) {
	fsys := newImportMapFS(map[string]string{"importmap.toml": `[[pin`})

	_, err := LoadImportMapConfig(fsys, "importmap.toml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing importmap")
}

func TestRenderImportMapTagBuildsImportsJSON(t *testing.T) {
	entries := []ResolvedEntry{
		{Name: "a", URL: "/static/a.js"},
		{Name: "b", URL: "/static/b.js"},
	}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)

	start := strings.Index(string(html), `<script type="importmap">`) + len(`<script type="importmap">`)
	end := strings.Index(string(html), `</script>`)
	require.Greater(t, end, start)
	payload := string(html)[start:end]

	var decoded struct {
		Imports map[string]string `json:"imports"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	require.Equal(t, map[string]string{"a": "/static/a.js", "b": "/static/b.js"}, decoded.Imports)
}

func TestRenderImportMapTagEscapesClosingScriptTag(t *testing.T) {
	dangerous := "/static/</script>a.js"
	entries := []ResolvedEntry{{Name: "a", URL: dangerous}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.NotContains(t, string(html), "</script>a.js")

	start := strings.Index(string(html), `<script type="importmap">`) + len(`<script type="importmap">`)
	end := strings.Index(string(html), `</script>`)
	require.Greater(t, end, start)

	var decoded struct {
		Imports map[string]string `json:"imports"`
	}
	require.NoError(t, json.Unmarshal([]byte(string(html)[start:end]), &decoded))
	require.Equal(t, dangerous, decoded.Imports["a"])
}

func TestRenderImportMapTagEmitsPreloadLinkWithCorrectAttribute(t *testing.T) {
	entries := []ResolvedEntry{{Name: "app", URL: "/static/app-abc.js", Preload: true}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.Contains(t, string(html), `<link rel="modulepreload" href="/static/app-abc.js">`)
}

func TestRenderImportMapTagOmitsPreloadLinkWhenNotPreload(t *testing.T) {
	entries := []ResolvedEntry{{Name: "app", URL: "/static/app-abc.js", Preload: false}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.NotContains(t, string(html), "<link")
}

func TestRenderImportMapTagAppendsApplicationBootstrapScript(t *testing.T) {
	entries := []ResolvedEntry{{Name: "application", URL: "/static/application-abc.js"}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.Contains(t, string(html), `<script type="module">import "application"</script>`)
}

func TestRenderImportMapTagOmitsApplicationBootstrapScriptWhenAbsent(t *testing.T) {
	entries := []ResolvedEntry{{Name: "other", URL: "/static/other-abc.js"}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.NotContains(t, string(html), `import "application"`)
}

func TestRenderImportMapTagPreloadAndApplicationCanCoincide(t *testing.T) {
	entries := []ResolvedEntry{{Name: "application", URL: "/static/application-abc.js", Preload: true}}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)
	require.Contains(t, string(html), `<link rel="modulepreload" href="/static/application-abc.js">`)
	require.Contains(t, string(html), `import "application"`)
}

func TestRenderImportMapTagPreservesInputEntryOrderForPreloadLinks(t *testing.T) {
	entries := []ResolvedEntry{
		{Name: "b", URL: "/static/b-abc.js", Preload: true},
		{Name: "a", URL: "/static/a-def.js", Preload: true},
	}

	html, err := RenderImportMapTag(entries)
	require.NoError(t, err)

	marker := "</script>\n"
	idx := strings.Index(string(html), marker)
	require.GreaterOrEqual(t, idx, 0)
	rest := string(html)[idx+len(marker):]

	bIdx := strings.Index(rest, "/static/b-abc.js")
	aIdx := strings.Index(rest, "/static/a-def.js")
	require.GreaterOrEqual(t, bIdx, 0)
	require.GreaterOrEqual(t, aIdx, 0)
	require.Less(t, bIdx, aIdx)
}

func TestJspmResolverGenerateReturnsImportsOnSuccess(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.MatchedBy(func(req *http.Request) bool {
		if req.Method != http.MethodPost || req.URL.String() != "https://api.jspm.io/generate" {
			return false
		}
		if req.Header.Get("Content-Type") != "application/json" {
			return false
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return false
		}
		var decoded jspmGenerateRequest
		if err := json.Unmarshal(body, &decoded); err != nil {
			return false
		}
		return len(decoded.Install) == 1 && decoded.Install[0] == "lit" &&
			decoded.FlattenScope &&
			len(decoded.Env) == 3 &&
			decoded.Env[0] == "browser" && decoded.Env[1] == "module" && decoded.Env[2] == "production"
	})).Return(jsonResponse(http.StatusOK, `{"map":{"imports":{"lit":"https://cdn/lit.js"}}}`), nil)

	resolver := &JspmResolver{httpClient: doer}
	imports, err := resolver.Generate("lit")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"lit": "https://cdn/lit.js"}, imports)
	doer.AssertExpectations(t)
}

func TestJspmResolverGenerateReturnsErrorOnNon200Status(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.Anything).Return(jsonResponse(http.StatusInternalServerError, "boom"), nil)

	resolver := &JspmResolver{httpClient: doer}
	_, err := resolver.Generate("lit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestJspmResolverGenerateReturnsErrorWhenResponseErrorFieldSet(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.Anything).Return(jsonResponse(http.StatusOK, `{"error":"package not found"}`), nil)

	resolver := &JspmResolver{httpClient: doer}
	_, err := resolver.Generate("lit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "package not found")
}

func TestJspmResolverGenerateReturnsErrorWhenImportsEmpty(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.Anything).Return(jsonResponse(http.StatusOK, `{"map":{"imports":{}}}`), nil)

	resolver := &JspmResolver{httpClient: doer}
	_, err := resolver.Generate("lit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "lit")
}

func TestJspmResolverGenerateReturnsErrorOnMalformedResponseJSON(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.Anything).Return(jsonResponse(http.StatusOK, `{not-json`), nil)

	resolver := &JspmResolver{httpClient: doer}
	_, err := resolver.Generate("lit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing jspm response")
}

func TestJspmResolverGenerateReturnsErrorWhenDoerFails(t *testing.T) {
	doer := &mockHTTPDoer{}
	doer.On("Do", mock.Anything).Return(nil, errors.New("connection refused"))

	resolver := &JspmResolver{httpClient: doer}
	_, err := resolver.Generate("lit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "calling jspm generate api")
}

func TestJspmResolverClientDefaultsToHTTPDefaultClientWhenNil(t *testing.T) {
	resolver := &JspmResolver{}
	require.Same(t, http.DefaultClient, resolver.client())
}

func TestDownloadReturnsBodyOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	data, err := Download(srv.URL)
	require.NoError(t, err)
	require.Equal(t, []byte("hello world"), data)
}

func TestDownloadReturnsErrorOnNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Download(srv.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), srv.URL)
}

func TestDownloadReturnsErrorOnUnreachableURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := Download(url)
	require.Error(t, err)
	require.Contains(t, err.Error(), "downloading")
}

func TestStripSourceMappingURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line with trailing newline",
			input: "code;\n//# sourceMappingURL=app.js.map\n",
			want:  "code;\n",
		},
		{
			name:  "no trailing newline",
			input: "code;\n//# sourceMappingURL=app.js.map",
			want:  "code;\n",
		},
		{
			name:  "indented marker",
			input: "code;\n  //# sourceMappingURL=app.js.map\n",
			want:  "code;\n",
		},
		{
			name:  "no match",
			input: "code;\nmore code;\n",
			want:  "code;\nmore code;\n",
		},
		{
			name:  "multiple occurrences",
			input: "a;\n//# sourceMappingURL=a.js.map\nb;\n//# sourceMappingURL=b.js.map\n",
			want:  "a;\nb;\n",
		},
		{
			name:  "extra whitespace before value",
			input: "code;\n//#  sourceMappingURL=app.js.map\n",
			want:  "code;\n",
		},
		{
			name:  "marker not alone on its line is untouched",
			input: "foo //# sourceMappingURL=x.map\n",
			want:  "foo //# sourceMappingURL=x.map\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StripSourceMappingURL([]byte(tc.input))
			require.Equal(t, tc.want, string(got))
		})
	}
}

func TestVendorFilename(t *testing.T) {
	tests := []struct {
		name      string
		specifier string
		url       string
		want      string
	}{
		{"plain package with js extension", "lit", "https://cdn.example.com/lit.js", "lit.js"},
		{"scoped package", "@lit-labs/ssr", "https://cdn.example.com/ssr.js", "lit-labs-ssr.js"},
		{"different extension", "lit", "https://cdn.example.com/lit.mjs", "lit.mjs"},
		{"no extension in basename defaults to js", "lit", "https://cdn.example.com/lit", "lit.js"},
		{"trailing slash url defaults to js", "lit", "https://cdn.example.com/", "lit.js"},
		{"unparseable url defaults to js", "lit", "://bad url", "lit.js"},
		{"multiple at and slash characters are all stripped", "@scope/name@2", "https://cdn.example.com/file.js", "scope-name2.js"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, VendorFilename(tc.specifier, tc.url))
		})
	}
}
