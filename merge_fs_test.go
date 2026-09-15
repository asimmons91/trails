package trails

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestMergeFSOpenFromFirstRoot(t *testing.T) {
	a := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}
	b := fstest.MapFS{"b.txt": &fstest.MapFile{Data: []byte("b")}}

	merged := MergeFS(a, b)

	data, err := fs.ReadFile(merged, "a.txt")
	require.NoError(t, err)
	require.Equal(t, "a", string(data))
}

func TestMergeFSOpenFromSecondRoot(t *testing.T) {
	a := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}
	b := fstest.MapFS{"b.txt": &fstest.MapFile{Data: []byte("b")}}

	merged := MergeFS(a, b)

	data, err := fs.ReadFile(merged, "b.txt")
	require.NoError(t, err)
	require.Equal(t, "b", string(data))
}

func TestMergeFSFirstRootShadowsSecond(t *testing.T) {
	a := fstest.MapFS{"shared.txt": &fstest.MapFile{Data: []byte("host")}}
	b := fstest.MapFS{"shared.txt": &fstest.MapFile{Data: []byte("spur")}}

	merged := MergeFS(a, b)

	data, err := fs.ReadFile(merged, "shared.txt")
	require.NoError(t, err)
	require.Equal(t, "host", string(data))
}

func TestMergeFSOpenNotFoundInAnyRoot(t *testing.T) {
	a := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}
	b := fstest.MapFS{"b.txt": &fstest.MapFile{Data: []byte("b")}}

	merged := MergeFS(a, b)

	_, err := fs.ReadFile(merged, "missing.txt")
	require.Error(t, err)
}

func TestMergeFSReadDirMergesEntriesAcrossRoots(t *testing.T) {
	a := fstest.MapFS{"views/root/index.gohtml": &fstest.MapFile{Data: []byte("root")}}
	b := fstest.MapFS{"views/posts/index.gohtml": &fstest.MapFile{Data: []byte("posts")}}

	merged := MergeFS(a, b)

	entries, err := fs.ReadDir(merged, "views")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Equal(t, []string{"posts", "root"}, names)
}

func TestMergeFSReadDirDedupesSameNameEntries(t *testing.T) {
	a := fstest.MapFS{"shared/a.txt": &fstest.MapFile{Data: []byte("host")}}
	b := fstest.MapFS{"shared/a.txt": &fstest.MapFile{Data: []byte("spur")}}

	merged := MergeFS(a, b)

	entries, err := fs.ReadDir(merged, "shared")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "a.txt", entries[0].Name())
}

func TestMergeFSReadDirNotFoundInAnyRoot(t *testing.T) {
	a := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}
	b := fstest.MapFS{"b.txt": &fstest.MapFile{Data: []byte("b")}}

	merged := MergeFS(a, b)

	_, err := fs.ReadDir(merged, "missing")
	require.Error(t, err)
}

func TestMergeFSGlobFindsFilesFromEitherRoot(t *testing.T) {
	a := fstest.MapFS{"layouts/application.gohtml": &fstest.MapFile{Data: []byte("layout")}}
	b := fstest.MapFS{"posts/index.gohtml": &fstest.MapFile{Data: []byte("posts")}}

	merged := MergeFS(a, b)

	matches, err := fs.Glob(merged, "layouts/*.gohtml")
	require.NoError(t, err)
	require.Equal(t, []string{"layouts/application.gohtml"}, matches)

	matches, err = fs.Glob(merged, "posts/*.gohtml")
	require.NoError(t, err)
	require.Equal(t, []string{"posts/index.gohtml"}, matches)
}
