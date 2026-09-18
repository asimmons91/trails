package trails

import (
	"fmt"
	"io/fs"

	"github.com/asimmons91/trails/assets"
	"github.com/asimmons91/trails/jobs"
)

// Spur is a mountable, self-contained app feature — its own views,
// assets, routes, and job registrations — that a host app wires in via
// Mount and the Register/MergeSpur* helpers rather than building
// directly into its own RouteBuilder. channels.Backend is the one Spur
// trails ships.
type Spur interface {
	// ViewFS returns the Spur's own view templates, merged under the
	// host's via MergeSpurViews, or nil if it has none.
	ViewFS() fs.FS
	// AssetsFS returns the Spur's own compiled assets, merged under the
	// host's via MergeSpurAssets/MergeSpurManifests, or nil if it has
	// none.
	AssetsFS() fs.FS
	// Routes registers the Spur's routes on g, which RegisterSpurRoutes
	// scopes to the Spur's Mount.Prefix.
	Routes(g *Group)
	// Jobs registers the Spur's job kinds into r, via RegisterSpurJobs.
	Jobs(r *jobs.Registry)
}

// Mount pairs a Spur with the URL prefix it's mounted under.
type Mount struct {
	Prefix string
	Spur   Spur
}

// MergeSpurViews merges hostViews with every mounted Spur's ViewFS
// (skipping any that returns nil), host first — so on a name collision,
// the host's own view wins over a Spur's.
func MergeSpurViews(hostViews fs.FS, mounts ...Mount) fs.FS {
	roots := make([]fs.FS, 0, len(mounts)+1)
	roots = append(roots, hostViews)
	for _, m := range mounts {
		if fsys := m.Spur.ViewFS(); fsys != nil {
			roots = append(roots, fsys)
		}
	}

	return MergeFS(roots...)
}

// MergeSpurAssets merges hostAssets with every mounted Spur's AssetsFS
// (skipping any that returns nil), host first — so on a name collision,
// the host's own asset wins over a Spur's.
func MergeSpurAssets(hostAssets fs.FS, mounts ...Mount) fs.FS {
	roots := make([]fs.FS, 0, len(mounts)+1)
	roots = append(roots, hostAssets)
	for _, m := range mounts {
		if fsys := m.Spur.AssetsFS(); fsys != nil {
			roots = append(roots, fsys)
		}
	}

	return MergeFS(roots...)
}

// MergeSpurManifests merges hostManifest with the manifest.json loaded
// from every mounted Spur's AssetsFS (skipping any that returns nil),
// host first — so on a key collision, the host's own manifest entry wins
// over a Spur's (see assets.MergeManifests).
func MergeSpurManifests(hostManifest assets.Manifest, mounts ...Mount) (assets.Manifest, error) {
	manifests := make([]assets.Manifest, 0, len(mounts)+1)
	manifests = append(manifests, hostManifest)

	for _, m := range mounts {
		fsys := m.Spur.AssetsFS()
		if fsys == nil {
			continue
		}

		manifest, err := assets.LoadManifest(fsys, "manifest.json")
		if err != nil {
			return nil, fmt.Errorf("trails: loading manifest for spur mounted at %q: %w", m.Prefix, err)
		}

		manifests = append(manifests, manifest)
	}

	return assets.MergeManifests(manifests...), nil
}

// RegisterSpurRoutes registers every mounted Spur's routes on r, each
// scoped to its own Mount.Prefix via r.WithGroup.
func RegisterSpurRoutes(r *Router, mounts ...Mount) {
	for _, m := range mounts {
		r.WithGroup(m.Prefix, m.Spur.Routes)
	}
}

// RegisterSpurJobs registers every mounted Spur's job kinds into reg.
func RegisterSpurJobs(reg *jobs.Registry, mounts ...Mount) {
	for _, m := range mounts {
		m.Spur.Jobs(reg)
	}
}

// RegisterSpurRunners collects the background Runner for every mounted Spur
// that has one (e.g. a channels.Backend wrapping a polling Broadcaster).
// Spurs with nothing to run are silently skipped. Pass the result as
// TrailOptions.Runners so Trail.Run starts and stops them alongside the HTTP
// server.
func RegisterSpurRunners(mounts ...Mount) []Runner {
	var runners []Runner
	for _, m := range mounts {
		if r, ok := m.Spur.(Runner); ok {
			runners = append(runners, r)
		}
	}
	return runners
}
