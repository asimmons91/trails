package assets

import (
	"fmt"
	"html/template"
	"path"
)

// FuncMap returns the template.FuncMap trails wires into every view:
// asset_path and asset_url both resolve a logical path to its
// prefix-joined digested URL via m (they are identical aliases — there is
// no separate absolute-URL variant despite the name), and import_maps
// returns importMapHTML verbatim, letting a layout template drop in the
// <script type="importmap"> tag built by RenderImportMapTag.
func FuncMap(m Manifest, prefix string, importMapHTML template.HTML) template.FuncMap {
	lookup := func(logical string) (string, error) {
		digested, ok := m.Path(logical)
		if !ok {
			return "", fmt.Errorf("assets: no manifest entry for %q", logical)
		}

		return path.Join(prefix, digested), nil
	}

	return template.FuncMap{
		"import_maps": func() template.HTML { return importMapHTML },
		"asset_path":  lookup,
		"asset_url":   lookup,
	}
}
