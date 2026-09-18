// Package assets implements trails' asset pipeline: fingerprinting built
// CSS/JS/map files for cache-busting (Compile) and resolving pins into a
// browser-native import map (ImportMap).
// A shared Manifest (logical path -> digested path)
// connects the two: Compile produces one, and both ImportMap.Resolve and
// the FuncMap template helpers (asset_path, asset_url, import_maps)
// resolve against it.
//
// Building assets is a two-stage step, normally run from the CLI:
//
//	assets.RunBundlerScripts(assets.BundlerConfig{Scripts: []string{"build"}})
//	manifest, err := assets.Compile("app/frontend/builds", "public/assets", "/assets")
//
// RunBundlerScripts runs the app's own JS build tool (esbuild, etc., via
// npm/yarn/pnpm/bun) to produce plain CSS/JS/map files; Compile then
// fingerprints that output. At request time, Trail loads the resulting
// manifest.json from TrailOptions.AssetsFS and, when AssetsStrategy is
// AssetsStrategyImportMap, an importmap.toml (LoadImportMapConfig) whose
// pins it resolves against that manifest and renders via
// RenderImportMapTag. AssetsStrategyBundler and AssetsStrategyNone skip
// the importmap step entirely; asset_path/asset_url stay available under
// either strategy.
//
// The compiled output directory is served with Router.Static, which
// bypasses trails' middleware chain entirely — fingerprinted asset
// requests never run through session, csrf, or any other Router.Use
// middleware.
package assets

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// Manifest maps a logical asset path (e.g. "app.css") to its fingerprinted
// output path (e.g. "app-3f2a1b.css"). It's the shared contract between
// Compile, which produces one, and ImportMap.Resolve/FuncMap, which
// resolve logical paths against one.
type Manifest map[string]string

// Path looks up logical's digested path. ok is false if logical has no
// entry.
func (m Manifest) Path(logical string) (string, bool) {
	p, ok := m[logical]

	return p, ok
}

// MergeManifests combines manifests into one, keeping the first entry
// seen for any logical path that appears in more than one — later
// manifests never override earlier ones, and collisions are silently
// dropped rather than reported. MergeSpurManifests relies on this to give
// a host app's own assets priority over any spur mounted alongside it.
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

// LoadManifest reads and parses the JSON manifest at path within fsys (as
// written by Compile).
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
