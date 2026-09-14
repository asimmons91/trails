package assets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectPackageManagerDefaultsToNPM(t *testing.T) {
	dir := t.TempDir()

	require.Equal(t, PackageManagerNPM, DetectPackageManager(dir))
}

func TestDetectPackageManagerFindsYarnLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "yarn.lock"), nil, 0o644))

	require.Equal(t, PackageManagerYarn, DetectPackageManager(dir))
}

func TestDetectPackageManagerFindsPnpmLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), nil, 0o644))

	require.Equal(t, PackageManagerPNPM, DetectPackageManager(dir))
}

func TestDetectPackageManagerFindsBunLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bun.lock"), nil, 0o644))

	require.Equal(t, PackageManagerBun, DetectPackageManager(dir))
}

func TestDetectPackageManagerFindsNPMLock(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), nil, 0o644))

	require.Equal(t, PackageManagerNPM, DetectPackageManager(dir))
}

func TestScriptArgsUsesRunSubcommandForNPMAndPNPM(t *testing.T) {
	require.Equal(t, []string{"run", "build"}, scriptArgs(PackageManagerNPM, "build"))
	require.Equal(t, []string{"run", "build"}, scriptArgs(PackageManagerPNPM, "build"))
}

func TestScriptArgsOmitsRunSubcommandForYarnAndBun(t *testing.T) {
	require.Equal(t, []string{"build"}, scriptArgs(PackageManagerYarn, "build"))
	require.Equal(t, []string{"build"}, scriptArgs(PackageManagerBun, "build"))
}

func TestScriptArgsAppendsExtraArgs(t *testing.T) {
	require.Equal(t, []string{"run", "build", "--", "--watch"}, scriptArgs(PackageManagerNPM, "build", "--", "--watch"))
}

func TestBundlerConfigManagerAutoDetects(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "yarn.lock"), nil, 0o644))

	cfg := BundlerConfig{Dir: dir}
	require.Equal(t, PackageManagerYarn, cfg.manager())
}

func TestBundlerConfigManagerHonorsExplicitChoice(t *testing.T) {
	cfg := BundlerConfig{Dir: t.TempDir(), Manager: PackageManagerBun}
	require.Equal(t, PackageManagerBun, cfg.manager())
}

func TestBundlerConfigDirDefaultsToCurrentDir(t *testing.T) {
	require.Equal(t, ".", BundlerConfig{}.dir())
}

func TestRunBundlerScriptsRunsEachScriptInOrder(t *testing.T) {
	cfg := BundlerConfig{Manager: PackageManager("true"), Scripts: []string{"build", "build:css"}}

	require.NoError(t, RunBundlerScripts(cfg))
}

func TestRunBundlerScriptsPropagatesFailure(t *testing.T) {
	cfg := BundlerConfig{Manager: PackageManager("false"), Scripts: []string{"build"}}

	err := RunBundlerScripts(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "running false")
}
