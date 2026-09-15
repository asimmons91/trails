package assets

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

type Manifest map[string]string

func (m Manifest) Path(logical string) (string, bool) {
	p, ok := m[logical]

	return p, ok
}

func MergeManifests(manifests ...Manifest) Manifest {
	merged := Manifest{}
	for _, m := range manifests {
		for k, v := range m {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}

	return merged
}

func LoadManifest(fsys fs.FS, path string) (Manifest, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("assets: reading manifest: %q: %w", path, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("assets: parsing manifest %q: %w", path, err)
	}

	return m, nil
}
