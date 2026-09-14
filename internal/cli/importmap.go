package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/asimmons91/trails/assets"
)

const importMapPath = "config/importmap.toml"
const publicDir = "pubic"
const assetsSourcePath = "app/frontend"
const assetsPrefix = "/assets"

type ImportmapCmd struct {
	Pin   PinCmd   `cmd:"" help:"Pin a package (and its dependencies) from jspm.org into the importmap."`
	Unpin UnpinCmd `cmd:"" help:"Remove a pin from the importmap."`
	Json  JsonCmd  `cmd:"" help:"Print the resolved importmap as JSON."`
}

type PinCmd struct {
	Name    string `arg:"" help:"Package to pin, e.g. lodash-es or lodash-es@4.17.21"`
	Preload bool   `help:"Preload this pin's dependency graph." default:"true"`
	Vendor  bool   `help:"Download resolved dependencies into the local vendor directory instead of referencing the CDN directly." default:"true" negatable:""`
}

func (c *PinCmd) Run() error {
	logger := slog.Default()

	im, err := assets.LoadImportMapConfig(os.DirFS("."), importMapPath)
	if err != nil {
		return err
	}

	resolver := &assets.JspmResolver{}
	resolved, err := resolver.Generate(c.Name)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", c.Name, err)
	}

	vendorDir := filepath.Join(assetsSourcePath, "vendor")

	for specifier, url := range resolved {
		pin := assets.Pin{Name: specifier, Preload: c.Preload}

		if c.Vendor {
			data, err := assets.Download(url)
			if err != nil {
				return fmt.Errorf("vendoring %q: %w", specifier, err)
			}
			data = assets.StripSourceMappingURL(data)

			filename := assets.VendorFilename(specifier, url)
			if err := os.MkdirAll(vendorDir, 0o755); err != nil {
				return fmt.Errorf("creating vendor dir: %w", err)
			}
			if err := os.WriteFile(filepath.Join(vendorDir, filename), data, 0o644); err != nil {
				return fmt.Errorf("writing vendored file for %q: %w", specifier, err)
			}

			pin.To = filename
		} else {
			pin.To = url
		}

		im.AddPin(pin)
		logger.Info("pinned", "name", specifier, "to", pin.To)
	}

	if err := im.Save(importMapPath); err != nil {
		return err
	}

	logger.Info("importmap updated", "path", importMapPath)
	return nil
}

type UnpinCmd struct {
	Name string `arg:"" help:"Name of the pin to remove"`
}

func (c *UnpinCmd) Run() error {
	logger := slog.Default()

	im, err := assets.LoadImportMapConfig(os.DirFS("."), importMapPath)
	if err != nil {
		return err
	}

	removed, ok := im.RemovePin(c.Name)
	if !ok {
		return fmt.Errorf("no pin named %q", c.Name)
	}

	if removed.To != "" && !isVendoredPinStillReferenced(im, removed.To) {
		if vendoredPath, ok := localVendorPath(assetsSourcePath, removed.To); ok {
			if err := os.Remove(vendoredPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing vendored file for %q: %w", c.Name, err)
			}
		}
	}

	if err := im.Save(importMapPath); err != nil {
		return err
	}

	logger.Info("unpinned", "name", c.Name)
	return nil
}

func isVendoredPinStillReferenced(im *assets.ImportMap, to string) bool {
	for _, p := range im.Pins {
		if p.To == to {
			return true
		}
	}
	return false
}

func localVendorPath(sourceDir, to string) (string, bool) {
	if to == "" || strings.HasPrefix(to, "http://") || strings.HasPrefix(to, "https://") {
		return "", false
	}
	return filepath.Join(sourceDir, "vendor", to), true
}

type JsonCmd struct{}

func (c *JsonCmd) Run() error {
	im, err := assets.LoadImportMapConfig(os.DirFS("."), importMapPath)
	if err != nil {
		return err
	}

	manifest, err := assets.LoadManifest(os.DirFS(publicDir), "manifest.json")
	if err != nil {
		return fmt.Errorf("loading asset manifest (run `mise run assets` first): %w", err)
	}

	entries, err := im.Resolve(manifest, assetsPrefix)
	if err != nil {
		return err
	}

	imports := make(map[string]string, len(entries))
	for _, e := range entries {
		imports[e.Name] = e.URL
	}

	out, err := json.MarshalIndent(struct {
		Imports map[string]string `json:"imports"`
	}{Imports: imports}, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(out))
	return nil
}
