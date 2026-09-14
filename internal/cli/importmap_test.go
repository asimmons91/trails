package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asimmons91/trails/assets"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func chdirTemp(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(orig))
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readImportMap(t *testing.T, path string) assets.ImportMap {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var im assets.ImportMap
	require.NoError(t, toml.Unmarshal(data, &im))
	return im
}

// stubTransport intercepts requests to api.jspm.io (the hardcoded jspm
// generate endpoint) and delegates everything else (e.g. vendor downloads
// against a local httptest.Server) to the original transport.
type stubTransport struct {
	base    http.RoundTripper
	handler func(req *http.Request) (*http.Response, error)
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Hostname() == "api.jspm.io" {
		return s.handler(req)
	}
	return s.base.RoundTrip(req)
}

func stubJspmTransport(t *testing.T, handler func(req *http.Request) (*http.Response, error)) {
	t.Helper()

	orig := http.DefaultTransport
	http.DefaultTransport = &stubTransport{base: orig, handler: handler}
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func jspmJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	fn()
	require.NoError(t, w.Close())
	os.Stdout = orig

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestIsVendoredPinStillReferenced(t *testing.T) {
	tests := []struct {
		name string
		im   *assets.ImportMap
		to   string
		want bool
	}{
		{
			name: "referenced by another pin",
			im:   &assets.ImportMap{Pins: []assets.Pin{{Name: "a", To: "lit.js"}, {Name: "b", To: "lit.js"}}},
			to:   "lit.js",
			want: true,
		},
		{
			name: "not referenced",
			im:   &assets.ImportMap{Pins: []assets.Pin{{Name: "a", To: "other.js"}}},
			to:   "lit.js",
			want: false,
		},
		{
			name: "no pins",
			im:   &assets.ImportMap{},
			to:   "lit.js",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isVendoredPinStillReferenced(tc.im, tc.to))
		})
	}
}

func TestLocalVendorPath(t *testing.T) {
	tests := []struct {
		name      string
		sourceDir string
		to        string
		wantPath  string
		wantOk    bool
	}{
		{
			name:      "empty to",
			sourceDir: "app/frontend",
			to:        "",
			wantOk:    false,
		},
		{
			name:      "http url",
			sourceDir: "app/frontend",
			to:        "http://cdn.example.com/lit.js",
			wantOk:    false,
		},
		{
			name:      "https url",
			sourceDir: "app/frontend",
			to:        "https://cdn.example.com/lit.js",
			wantOk:    false,
		},
		{
			name:      "local vendored file",
			sourceDir: "app/frontend",
			to:        "lit.js",
			wantPath:  filepath.Join("app/frontend", "vendor", "lit.js"),
			wantOk:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := localVendorPath(tc.sourceDir, tc.to)
			require.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				require.Equal(t, tc.wantPath, got)
			}
		})
	}
}

func TestPinCmdRunReturnsErrorWhenImportMapConfigMissing(t *testing.T) {
	chdirTemp(t)

	cmd := PinCmd{Name: "lit"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "importmap")
}

func TestPinCmdRunReturnsErrorWhenResolverFails(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, "")

	stubJspmTransport(t, func(req *http.Request) (*http.Response, error) {
		return jspmJSONResponse(http.StatusOK, `{"error":"package not found"}`), nil
	})

	cmd := PinCmd{Name: "badpkg"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolving")
}

func TestPinCmdRunVendorsAndWritesPinWhenVendorTrue(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, "")

	vendorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("console.log('lit');\n"))
	}))
	defer vendorSrv.Close()

	stubJspmTransport(t, func(req *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"map":{"imports":{"lit":%q}}}`, vendorSrv.URL+"/lit.js")
		return jspmJSONResponse(http.StatusOK, body), nil
	})

	cmd := PinCmd{Name: "lit", Preload: true, Vendor: true}
	err := cmd.Run()
	require.NoError(t, err)

	vendoredPath := filepath.Join(assetsSourcePath, "vendor", "lit.js")
	data, err := os.ReadFile(vendoredPath)
	require.NoError(t, err)
	require.Equal(t, "console.log('lit');\n", string(data))

	im := readImportMap(t, importMapPath)
	require.Equal(t, []assets.Pin{{Name: "lit", To: "lit.js", Preload: true}}, im.Pins)
}

func TestPinCmdRunSkipsVendoringWhenVendorFalse(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, "")

	const externalURL = "https://cdn.example.com/lit.js"
	stubJspmTransport(t, func(req *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"map":{"imports":{"lit":%q}}}`, externalURL)
		return jspmJSONResponse(http.StatusOK, body), nil
	})

	cmd := PinCmd{Name: "lit", Vendor: false}
	err := cmd.Run()
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(assetsSourcePath, "vendor"))
	require.True(t, os.IsNotExist(statErr))

	im := readImportMap(t, importMapPath)
	require.Equal(t, []assets.Pin{{Name: "lit", To: externalURL}}, im.Pins)
}

func TestPinCmdRunReturnsErrorWhenDownloadFails(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, "")

	vendorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer vendorSrv.Close()

	stubJspmTransport(t, func(req *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"map":{"imports":{"lit":%q}}}`, vendorSrv.URL+"/lit.js")
		return jspmJSONResponse(http.StatusOK, body), nil
	})

	cmd := PinCmd{Name: "lit", Vendor: true}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "vendoring")
}

func TestUnpinCmdRunReturnsErrorWhenImportMapConfigMissing(t *testing.T) {
	chdirTemp(t)

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "importmap")
}

func TestUnpinCmdRunReturnsErrorWhenPinNotFound(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "other"
`)

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), `no pin named "lit"`)
}

func TestUnpinCmdRunRemovesPinAndDeletesVendoredFileWhenNotReferenced(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "lit"
to = "lit.js"
`)
	vendoredPath := filepath.Join(assetsSourcePath, "vendor", "lit.js")
	writeFile(t, vendoredPath, "console.log('lit');\n")

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.NoError(t, err)

	im := readImportMap(t, importMapPath)
	require.Empty(t, im.Pins)

	_, statErr := os.Stat(vendoredPath)
	require.True(t, os.IsNotExist(statErr))
}

func TestUnpinCmdRunKeepsVendoredFileWhenStillReferencedByAnotherPin(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "lit"
to = "lit.js"

[[pin]]
name = "lit-alias"
to = "lit.js"
`)
	vendoredPath := filepath.Join(assetsSourcePath, "vendor", "lit.js")
	writeFile(t, vendoredPath, "console.log('lit');\n")

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.NoError(t, err)

	im := readImportMap(t, importMapPath)
	require.Equal(t, []assets.Pin{{Name: "lit-alias", To: "lit.js"}}, im.Pins)

	_, statErr := os.Stat(vendoredPath)
	require.NoError(t, statErr)
}

func TestUnpinCmdRunSkipsFileRemovalForExternalPin(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "lit"
to = "https://cdn.example.com/lit.js"
`)

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.NoError(t, err)

	im := readImportMap(t, importMapPath)
	require.Empty(t, im.Pins)
}

func TestUnpinCmdRunIgnoresAlreadyMissingVendoredFile(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "lit"
to = "lit.js"
`)

	cmd := UnpinCmd{Name: "lit"}
	err := cmd.Run()
	require.NoError(t, err)

	im := readImportMap(t, importMapPath)
	require.Empty(t, im.Pins)
}

func TestJsonCmdRunReturnsErrorWhenImportMapConfigMissing(t *testing.T) {
	chdirTemp(t)

	cmd := JsonCmd{}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "importmap")
}

func TestJsonCmdRunReturnsErrorWhenManifestMissing(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, "")

	cmd := JsonCmd{}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading asset manifest")
}

func TestJsonCmdRunPrintsResolvedImportsAsJSON(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "application"
preload = true
`)
	writeFile(t, filepath.Join(publicDir, "manifest.json"), `{"application.js":"application-abc123.js"}`)

	cmd := JsonCmd{}
	var runErr error
	out := captureStdout(t, func() {
		runErr = cmd.Run()
	})
	require.NoError(t, runErr)

	var decoded struct {
		Imports map[string]string `json:"imports"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	require.Equal(t, map[string]string{"application": "/assets/application-abc123.js"}, decoded.Imports)
}

func TestJsonCmdRunReturnsErrorWhenResolveFails(t *testing.T) {
	chdirTemp(t)
	writeFile(t, importMapPath, `
[[pin]]
name = "missing"
`)
	writeFile(t, filepath.Join(publicDir, "manifest.json"), `{}`)

	cmd := JsonCmd{}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no manifest entry")
}
