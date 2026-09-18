package trails

import (
	"io/fs"
	"sort"
)

// MergeFS presents roots as one fs.FS: Open returns the first root that
// has the requested name (later roots are only consulted if earlier ones
// return an error), while ReadDir instead unions every root's listing for
// a directory, first-root-wins on a name collision. There is no Sub,
// Stat, or Glob of its own — those work via the generic io/fs helpers
// built on Open/ReadDir.
func MergeFS(roots ...fs.FS) fs.FS {
	return &mergedFS{roots: roots}
}

type mergedFS struct {
	roots []fs.FS
}

func (m *mergedFS) Open(name string) (fs.File, error) {
	var firstErr error
	for _, root := range m.roots {
		f, err := root.Open(name)
		if err == nil {
			return f, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	return nil, firstErr
}

func (m *mergedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	seen := map[string]fs.DirEntry{}
	var firstErr error
	found := false

	for _, root := range m.roots {
		entries, err := fs.ReadDir(root, name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		found = true
		for _, entry := range entries {
			if _, ok := seen[entry.Name()]; !ok {
				seen[entry.Name()] = entry
			}
		}
	}

	if !found {
		return nil, firstErr
	}

	merged := make([]fs.DirEntry, 0, len(seen))
	for _, entry := range seen {
		merged = append(merged, entry)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Name() < merged[j].Name() })

	return merged, nil
}
