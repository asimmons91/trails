package cli

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

const migrationsDir = "db/migrations"
const migrateBootstrapDir = ".trails-migrate"

type MigrateCmd struct {
	Create CreateCmd `cmd:"" help:"Generate a new migration."`
	Up     UpCmd     `cmd:"" help:"Run all pending migrations."`
	Down   DownCmd   `cmd:"" help:"Roll back the most recently applied migration."`
}

type CreateCmd struct {
	Name string `arg:"" help:"Name of the new migration, e.g. create_widgets"`
}

var migrationFileTemplate = template.Must(template.New("migration").Parse(`package migrations

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

func init() {
	migrate.Register(migrate.Migration{
		ID: "{{.ID}}",
		Migrate: func(ctx context.Context, m *migrate.Migrator) error {
			return nil
		},
		Rollback: func(ctx context.Context, m *migrate.Migrator) error {
			return nil
		},
	})
}
`))

var migrateSlugInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)

func migrateSlugify(name string) string {
	slug := migrateSlugInvalidChars.ReplaceAllString(strings.ToLower(name), "_")
	return strings.Trim(slug, "_")
}

func (c *CreateCmd) Run() error {
	logger := slog.Default()

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return fmt.Errorf("migrate: creating %s: %w", migrationsDir, err)
	}

	id := time.Now().Format("20060102150405")
	slug := migrateSlugify(c.Name)
	if slug == "" {
		return fmt.Errorf("migrate: invalid name %q", c.Name)
	}

	path := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.go", id, slug))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("migrate: creating migration file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := migrationFileTemplate.Execute(f, struct{ ID string }{ID: id}); err != nil {
		return fmt.Errorf("migrate: rendering migration template: %w", err)
	}

	logger.Info("created migration", "path", path)
	return nil
}

type UpCmd struct{}

func (u *UpCmd) Run() error {
	return runMigrateBootstrap("up")
}

type DownCmd struct{}

func (d *DownCmd) Run() error {
	return runMigrateBootstrap("down")
}

var migrateBootstrapTemplate = template.Must(template.New("bootstrap").Parse(`package main

import (
	"context"
	"fmt"
	"os"

	"github.com/asimmons91/trails/pack/migrate"
	projectdb "{{.ModulePath}}/db"
	_ "{{.ModulePath}}/db/migrations"
)

func main() {
	ctx := context.Background()

	db, err := projectdb.Connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if os.Args[1] == "down" {
		err = migrate.Down(ctx, db)
	} else {
		err = migrate.Up(ctx, db)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`))

func readMigrateModulePath(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("migrate: reading go.mod: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("migrate: reading go.mod: %w", err)
	}

	return "", fmt.Errorf("migrate: go.mod: no module directive found")
}

func runMigrateBootstrap(direction string) error {
	logger := slog.Default()

	modulePath, err := readMigrateModulePath(".")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(migrateBootstrapDir, 0o755); err != nil {
		return fmt.Errorf("migrate: creating %s: %w", migrateBootstrapDir, err)
	}
	defer func() { _ = os.RemoveAll(migrateBootstrapDir) }()

	f, err := os.Create(filepath.Join(migrateBootstrapDir, "main.go"))
	if err != nil {
		return fmt.Errorf("migrate: writing migrate bootstrap: %w", err)
	}

	err = migrateBootstrapTemplate.Execute(f, struct{ ModulePath string }{ModulePath: modulePath})
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("migrate: rendering migrate bootstrap: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("migrate: writing migrate bootstrap: %w", closeErr)
	}

	cmd := exec.Command("go", "run", "./"+migrateBootstrapDir, direction)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("migrate: running migrations: %w", err)
	}

	logger.Info("migrations complete", "direction", direction)
	return nil
}
