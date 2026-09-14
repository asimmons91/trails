package assets

import (
	"fmt"
	"html/template"
	"path"
)

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
