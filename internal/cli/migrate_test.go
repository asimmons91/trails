package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateSlugify(t *testing.T) {
	cases := map[string]string{
		"Create Widgets":   "create_widgets",
		"create_widgets":   "create_widgets",
		"  spaced  out  ":  "spaced_out",
		"!!!":              "",
		"Add-FK--to_users": "add_fk_to_users",
	}

	for in, want := range cases {
		assert.Equal(t, want, migrateSlugify(in), "migrateSlugify(%q)", in)
	}
}

func TestCreateCmdRun_ScaffoldsMigrationFile(t *testing.T) {
	chdirTemp(t)

	cmd := CreateCmd{Name: "Create Widgets"}
	require.NoError(t, cmd.Run())

	entries, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	name := entries[0].Name()
	assert.True(t, strings.HasSuffix(name, "_create_widgets.go"), "name = %q", name)
	assert.Len(t, strings.SplitN(name, "_", 2)[0], 14, "expected a 14-digit timestamp prefix in %q", name)

	data, err := os.ReadFile(filepath.Join(migrationsDir, name))
	require.NoError(t, err)
	contents := string(data)

	assert.Contains(t, contents, "package migrations")
	assert.Contains(t, contents, `"github.com/asimmons91/trails/pack/migrate"`)
	assert.Contains(t, contents, "migrate.Register(migrate.Migration{")
	assert.Contains(t, contents, strings.SplitN(name, "_", 2)[0])
}

func TestCreateCmdRun_InvalidName_ReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := CreateCmd{Name: "!!!"}
	err := cmd.Run()
	require.Error(t, err)
}

func TestReadMigrateModulePath(t *testing.T) {
	chdirTemp(t)

	writeFile(t, "go.mod", "module github.com/example/widgets\n\ngo 1.27\n")

	path, err := readMigrateModulePath(".")
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/widgets", path)
}

func TestReadMigrateModulePath_MissingGoMod_ReturnsError(t *testing.T) {
	chdirTemp(t)

	_, err := readMigrateModulePath(".")
	require.Error(t, err)
}

func TestReadMigrateModulePath_NoModuleDirective_ReturnsError(t *testing.T) {
	chdirTemp(t)

	writeFile(t, "go.mod", "go 1.27\n")

	_, err := readMigrateModulePath(".")
	require.Error(t, err)
}
