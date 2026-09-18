package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PackageManager identifies the CLI used to run package.json scripts.
type PackageManager string

// Package managers recognized by DetectPackageManager. PackageManagerNPM
// is also the default when no lockfile is found.
const (
	PackageManagerNPM  PackageManager = "npm"
	PackageManagerYarn PackageManager = "yarn"
	PackageManagerPNPM PackageManager = "pnpm"
	PackageManagerBun  PackageManager = "bun"
)

// BundlerConfig configures RunBundlerScripts.
type BundlerConfig struct {
	// Dir is the working directory containing package.json. Empty
	// defaults to ".".
	Dir string
	// Manager is the package manager CLI to invoke. Empty auto-detects
	// via DetectPackageManager(Dir).
	Manager PackageManager
	// Scripts are the package.json scripts to run, in order. The first
	// one that fails stops the rest.
	Scripts []string
}

var lockFileManagers = []struct {
	file    string
	manager PackageManager
}{
	{"yarn.lock", PackageManagerYarn},
	{"pnpm-lock.yaml", PackageManagerPNPM},
	{"bun.lock", PackageManagerBun},
	{"package-lock.json", PackageManagerNPM},
}

// DetectPackageManager inspects dir for a lockfile — yarn.lock,
// pnpm-lock.yaml, bun.lock, or package-lock.json, checked in that
// priority order — and returns the corresponding PackageManager. If dir
// contains more than one, the first match in that order wins; if none is
// found, it defaults to PackageManagerNPM.
func DetectPackageManager(dir string) PackageManager {
	for _, lf := range lockFileManagers {
		if _, err := os.Stat(filepath.Join(dir, lf.file)); err == nil {
			return lf.manager
		}
	}

	return PackageManagerNPM
}

func (c BundlerConfig) dir() string {
	if c.Dir == "" {
		return "."
	}

	return c.Dir
}

func (c BundlerConfig) manager() PackageManager {
	if c.Manager != "" {
		return c.Manager
	}

	return DetectPackageManager(c.dir())
}

func scriptArgs(manager PackageManager, script string, extra ...string) []string {
	var args []string

	switch manager {
	case PackageManagerYarn, PackageManagerBun:
		args = append([]string{script}, extra...)
	default:
		args = append([]string{"run", script}, extra...)
	}

	return args
}

func runCommand(dir string, manager PackageManager, args []string) error {
	cmd := exec.Command(string(manager), args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("assets: running %s %s: %w", manager, args, err)
	}

	return nil
}

// RunBundlerScripts runs cfg.Scripts in order via cfg's package manager
// (see BundlerConfig), stopping at the first script that fails. Each
// script's stdout/stderr are passed through to the current process's own
// stdout/stderr, not captured.
func RunBundlerScripts(cfg BundlerConfig) error {
	manager := cfg.manager()
	dir := cfg.dir()

	for _, script := range cfg.Scripts {
		if err := runCommand(dir, manager, scriptArgs(manager, script)); err != nil {
			return err
		}
	}

	return nil
}
