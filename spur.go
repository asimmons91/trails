package trails

import (
	"fmt"
	"io/fs"

	"github.com/asimmons91/trails/assets"
	"github.com/asimmons91/trails/jobs"
)

type Spur interface {
	ViewFS() fs.FS
	AssetsFS() fs.FS
	Routes(g *Group)
	Jobs(r *jobs.Registry)
}

type Mount struct {
	Prefix string
	Spur   Spur
}

func MergeSpurViews(hostViews fs.FS, mounts ...Mount) fs.FS {
	roots := make([]fs.FS, 0, len(mounts)+1)
	roots = append(roots, hostViews)
	for _, m := range mounts {
		roots = append(roots, m.Spur.ViewFS())
	}

	return MergeFS(roots...)
}

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

func RegisterSpurRoutes(r *Router, mounts ...Mount) {
	for _, m := range mounts {
		r.WithGroup(m.Prefix, m.Spur.Routes)
	}
}

func RegisterSpurJobs(reg *jobs.Registry, mounts ...Mount) {
	for _, m := range mounts {
		m.Spur.Jobs(reg)
	}
}
