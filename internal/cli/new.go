package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/asimmons91/trails/assets"
	"github.com/asimmons91/trails/internal/cli/newtemplate"
)

const newTemplatesRoot = "templates"
const newTemplateSuffix = ".tmpl"
const newAppNamePlaceholder = "APPNAME"
const newExampleMigrationSrc = "db/migrations/create_examples.go.tmpl"
const newExecutableFile = ".mise/tasks/fmt"
const trailsGoVersion = "1.27"

var newVariantDirRE = regexp.MustCompile(`^__([a-z]+)__$`)

type NewCmd struct {
	Name     string `arg:"" help:"Name of the new application (also used as its directory name)."`
	Module   string `help:"Go module path for the new app. Defaults to the bare app name if omitted."`
	Dir      string `help:"Directory to create the app in." default:"."`
	Database string `help:"Database driver to use." enum:"sqlite,postgres,mysql" default:"sqlite"`
	Assets   string `help:"Frontend asset strategy." enum:"importmap,bundler" default:"importmap"`
	SkipGit  bool   `help:"Skip git init and the initial commit."`
}

type newTemplateData struct {
	AppName        string
	ModulePath     string
	GoVersion      string
	MigrationID    string
	Database       string
	AssetsStrategy string
}

func (c *NewCmd) Run() error {
	logger := slog.Default()

	if c.Name == "" || c.Name == "." || c.Name == ".." || strings.ContainsAny(c.Name, `/\`) {
		return fmt.Errorf("new: invalid app name %q", c.Name)
	}

	target := filepath.Join(c.Dir, c.Name)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("new: %s already exists", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("new: checking %s: %w", target, err)
	}

	modulePath := c.Module
	if modulePath == "" {
		modulePath = c.Name
	}

	data := newTemplateData{
		AppName:        c.Name,
		ModulePath:     modulePath,
		GoVersion:      trailsGoVersion,
		MigrationID:    time.Now().Format("20060102150405"),
		Database:       c.Database,
		AssetsStrategy: c.Assets,
	}

	if err := writeNewTemplateTree(target, data); err != nil {
		_ = os.RemoveAll(target)
		return fmt.Errorf("new: generating %s: %w", target, err)
	}
	logger.Info("generated app", "path", target, "database", data.Database, "assets", data.AssetsStrategy)

	if out, err := runIn(target, "gofmt", "-w", "."); err != nil {
		logger.Warn("new: `gofmt` failed, run it manually inside the app directory", "error", err, "output", out)
	}

	if out, err := runIn(target, "go", "mod", "tidy"); err != nil {
		logger.Warn("new: `go mod tidy` failed, run it manually inside the app directory", "error", err, "output", out)
	}

	if data.AssetsStrategy == "bundler" {
		runNewAssetsBuild(logger, target)
	}

	if !c.SkipGit {
		runNewGitInit(logger, target)
	}

	printNewNextSteps(target, modulePath, c.Module == "")

	return nil
}

// newVariantTag reports whether name is a variant-directory marker
// (e.g. "__postgres__") and, if so, the tag inside it.
func newVariantTag(name string) (string, bool) {
	m := newVariantDirRE.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// stripVariantSegments removes any path segment that is a variant-directory
// marker, e.g. "db/__postgres__/db.go.tmpl" -> "db/db.go.tmpl". Only ever
// applied to paths that were actually walked, so every segment it strips is
// already known to match the current selection.
func stripVariantSegments(rel string) string {
	parts := strings.Split(rel, string(filepath.Separator))
	kept := parts[:0]
	for _, p := range parts {
		if _, ok := newVariantTag(p); ok {
			continue
		}
		kept = append(kept, p)
	}
	return filepath.Join(kept...)
}

func writeNewTemplateTree(target string, data newTemplateData) error {
	selected := map[string]bool{
		data.Database:       true,
		data.AssetsStrategy: true,
	}

	return fs.WalkDir(newtemplate.FS, newTemplatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == newTemplatesRoot {
			return nil
		}

		rel, err := filepath.Rel(newTemplatesRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.FromSlash(rel)
		rel = strings.ReplaceAll(rel, newAppNamePlaceholder, data.AppName)

		if d.IsDir() {
			if tag, ok := newVariantTag(d.Name()); ok {
				if !selected[tag] {
					return fs.SkipDir
				}
				return nil
			}

			return os.MkdirAll(filepath.Join(target, stripVariantSegments(rel)), 0o755)
		}

		destRel := stripVariantSegments(rel)
		isTemplate := strings.HasSuffix(destRel, newTemplateSuffix)
		if isTemplate {
			destRel = strings.TrimSuffix(destRel, newTemplateSuffix)
		}

		srcRelSlash := filepath.ToSlash(strings.TrimPrefix(path, newTemplatesRoot+"/"))
		if srcRelSlash == newExampleMigrationSrc {
			destRel = filepath.Join("db", "migrations", fmt.Sprintf("%s_create_examples.go", data.MigrationID))
		}

		destPath := filepath.Join(target, destRel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		content, err := fs.ReadFile(newtemplate.FS, path)
		if err != nil {
			return err
		}

		perm := os.FileMode(0o644)
		if filepath.ToSlash(rel) == newExecutableFile {
			perm = 0o755
		}

		if isTemplate {
			tmpl, err := template.New(d.Name()).Parse(string(content))
			if err != nil {
				return fmt.Errorf("parsing template %s: %w", path, err)
			}

			f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			return tmpl.Execute(f, data)
		}

		return os.WriteFile(destPath, content, perm)
	})
}

func runIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func insideGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func runNewGitInit(logger *slog.Logger, target string) {
	if insideGitRepo(target) {
		logger.Info("new: skipping git init, already inside a git repository")
		return
	}

	if _, err := runIn(target, "git", "init"); err != nil {
		logger.Warn("new: `git init` failed, run it manually", "error", err)
		return
	}

	if _, err := runIn(target, "git", "add", "-A"); err != nil {
		logger.Warn("new: `git add -A` failed, run it manually", "error", err)
		return
	}

	if _, err := runIn(target, "git", "commit", "-m", "Initial commit from trails new"); err != nil {
		logger.Warn("new: initial commit failed, run it manually", "error", err)
	}
}

// runNewAssetsBuild best-effort compiles the bundler-strategy starter
// frontend during generation so the app boots with real assets immediately.
// Any failure leaves the pre-seeded placeholder assets in place.
func runNewAssetsBuild(logger *slog.Logger, target string) {
	if _, err := exec.LookPath("npm"); err != nil {
		logger.Warn("new: npm not found, using placeholder assets; run `mise run deps && mise run assets` manually")
		return
	}

	if out, err := runIn(target, "npm", "install"); err != nil {
		logger.Warn("new: `npm install` failed, using placeholder assets; run `mise run deps && mise run assets` manually", "error", err, "output", out)
		return
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		logger.Warn("new: resolving app path for asset build failed, run `mise run assets` manually", "error", err)
		return
	}

	if err := assets.RunBundlerScripts(assets.BundlerConfig{
		Dir:     absTarget,
		Scripts: []string{"build:js", "build:css"},
	}); err != nil {
		logger.Warn("new: asset build scripts failed, using placeholder assets; run `mise run assets` manually", "error", err)
		return
	}

	source := filepath.Join(absTarget, "app", "frontend", "builds")
	output := filepath.Join(absTarget, "public", "assets")
	if _, err := assets.Compile(source, output, "/assets"); err != nil {
		logger.Warn("new: fingerprinting assets failed, using placeholder assets; run `mise run assets` manually", "error", err)
		return
	}

	logger.Info("new: assets built")
}

func printNewNextSteps(target, modulePath string, moduleDefaulted bool) {
	fmt.Printf("\nYour new trails app is ready in %s/\n\n", target)
	fmt.Printf("  cd %s\n", target)
	fmt.Printf("  mise install\n")
	fmt.Printf("  trails migrate up\n")
	fmt.Printf("  mise run dev\n\n")

	if moduleDefaulted {
		fmt.Printf("Using module path %q (no --module given). Pass --module github.com/you/%s to publish this app.\n\n", modulePath, filepath.Base(target))
	}

	fmt.Println("See README.md for next steps (adding a frontend bundler, jobs, channels, ...).")
}
