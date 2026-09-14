package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type PackageManager string

const (
	PackageManagerNPM  PackageManager = "npm"
	PackageManagerYarn PackageManager = "yarn"
	PackageManagerPNPM PackageManager = "pnpm"
	PackageManagerBun  PackageManager = "bun"
)

type BundlerConfig struct {
	Dir     string
	Manager PackageManager
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
