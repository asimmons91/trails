package assets

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFuncMapImportMapsReturnsHTMLVerbatim(t *testing.T) {
	want := template.HTML(`<script>x</script>`)
	funcs := FuncMap(Manifest{}, "/static", want)

	got := funcs["import_maps"].(func() template.HTML)()
	require.Equal(t, want, got)
}

func TestFuncMapAssetPathResolvesJoinedPrefixPath(t *testing.T) {
	m := Manifest{"app.css": "app-abc123.css"}
	funcs := FuncMap(m, "/static", "")

	got, err := funcs["asset_path"].(func(string) (string, error))("app.css")
	require.NoError(t, err)
	require.Equal(t, "/static/app-abc123.css", got)
}

func TestFuncMapAssetURLResolvesJoinedPrefixPath(t *testing.T) {
	m := Manifest{"app.css": "app-abc123.css"}
	funcs := FuncMap(m, "/static", "")

	got, err := funcs["asset_url"].(func(string) (string, error))("app.css")
	require.NoError(t, err)
	require.Equal(t, "/static/app-abc123.css", got)
}

func TestFuncMapAssetPathReturnsErrorForMissingLogicalName(t *testing.T) {
	funcs := FuncMap(Manifest{}, "/static", "")

	_, err := funcs["asset_path"].(func(string) (string, error))("missing.css")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing.css")
}

func TestFuncMapAssetURLReturnsErrorForMissingLogicalName(t *testing.T) {
	funcs := FuncMap(Manifest{}, "/static", "")

	_, err := funcs["asset_url"].(func(string) (string, error))("missing.css")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing.css")
}

func TestFuncMapAssetPathJoinsPrefixCleanly(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"empty prefix", "", "app-abc123.css"},
		{"trailing slash prefix", "/static/", "/static/app-abc123.css"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Manifest{"app.css": "app-abc123.css"}
			funcs := FuncMap(m, tc.prefix, "")

			got, err := funcs["asset_path"].(func(string) (string, error))("app.css")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
