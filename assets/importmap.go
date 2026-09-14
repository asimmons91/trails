package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Pin struct {
	Name    string `toml:"name"`
	To      string `toml:"to,omitempty"`
	Preload bool   `toml:"preload"`
}

type PinAll struct {
	Dir     string `toml:"dir"`
	Under   string `toml:"under"`
	Preload bool   `toml:"preload"`
	Pattern string `toml:"pattern,omitempty"`
}

type ResolvedEntry struct {
	Name    string
	URL     string
	Preload bool
}

type ImportMap struct {
	Pins    []Pin    `toml:"pin"`
	PinAlls []PinAll `toml:"pin_all"`
}

func (p *Pin) resolve(m Manifest, prefix string) (ResolvedEntry, error) {
	if p.To == "" {
		digested, ok := m.Path(p.Name + ".js")
		if !ok {
			return ResolvedEntry{}, fmt.Errorf("assets: importmap pin %q: no manifest entry for %q", p.Name, p.Name+".js")
		}
		return ResolvedEntry{
			Name:    p.Name,
			URL:     path.Join(prefix, digested),
			Preload: p.Preload,
		}, nil
	}

	if isExternal(p.To) {
		return ResolvedEntry{
			Name:    p.Name,
			URL:     p.To,
			Preload: p.Preload,
		}, nil
	}

	key := path.Join("vendor", p.To)
	digested, ok := m.Path(key)
	if !ok {
		return ResolvedEntry{}, fmt.Errorf("assets: importmap pin %q: no manifest entry for %q", p.Name, key)
	}

	return ResolvedEntry{
		Name:    p.Name,
		URL:     path.Join(prefix, digested),
		Preload: p.Preload,
	}, nil

}

func (pa *PinAll) resolve(m Manifest, prefix string) ([]ResolvedEntry, error) {
	pattern := pa.Pattern
	if pattern == "" {
		pattern = "**/*.js"
	}
	recursive := strings.HasPrefix(pattern, "**/")
	pattern = strings.TrimPrefix(pattern, "**/")
	dirPrefix := strings.TrimSuffix(pa.Dir, "/") + "/"

	var entries []ResolvedEntry
	for key, digested := range m {
		rel, ok := strings.CutPrefix(key, dirPrefix)
		if !ok {
			continue
		}

		candidate := rel
		if recursive {
			candidate = path.Base(rel)
		}

		matched, err := path.Match(pattern, candidate)
		if err != nil {
			return nil, fmt.Errorf("assets: pin_all %q: bad pattern %q: %w", pa.Dir, pa.Pattern, err)
		}
		if !matched {
			continue
		}

		name := path.Join(pa.Under, strings.TrimSuffix(rel, path.Ext(rel)))
		entries = append(entries, ResolvedEntry{
			Name:    name,
			URL:     path.Join(prefix, digested),
			Preload: pa.Preload,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func (i *ImportMap) Save(path string) error {
	data, err := toml.Marshal(*i)
	if err != nil {
		return fmt.Errorf("assets: encoding importmap: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("assets: writing importmap: %q: %w", path, err)
	}

	return nil
}

func (i *ImportMap) AddPin(p Pin) {
	for idx, existing := range i.Pins {
		if existing.Name == p.Name {
			i.Pins[idx] = p
			return
		}
	}

	i.Pins = append(i.Pins, p)
}

func (i *ImportMap) RemovePin(name string) (Pin, bool) {
	for idx, existing := range i.Pins {
		if existing.Name == name {
			i.Pins = append(i.Pins[:idx], i.Pins[idx+1:]...)
			return existing, true
		}
	}

	return Pin{}, false
}

func (i *ImportMap) Resolve(m Manifest, prefix string) ([]ResolvedEntry, error) {
	entries := make([]ResolvedEntry, 0, len(i.Pins))

	for _, p := range i.Pins {
		entry, err := p.resolve(m, prefix)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	for _, pa := range i.PinAlls {
		resolved, err := pa.resolve(m, prefix)
		if err != nil {
			return nil, err
		}
		entries = append(entries, resolved...)
	}

	return entries, nil
}

func LoadImportMapConfig(fsys fs.FS, path string) (*ImportMap, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("assets: reading importmap %q: %w", path, err)
	}

	var im ImportMap
	if err := toml.Unmarshal(data, &im); err != nil {
		return nil, fmt.Errorf("assets: parsing importmap %q: %w", path, err)
	}

	return &im, nil
}

func RenderImportMapTag(entries []ResolvedEntry) (template.HTML, error) {
	imports := make(map[string]string, len(entries))
	for _, e := range entries {
		imports[e.Name] = e.URL
	}

	mapJson, err := json.Marshal(struct {
		Imports map[string]string `json:"imports"`
	}{Imports: imports})
	if err != nil {
		return "", fmt.Errorf("assets encoding importmap json: %w", err)
	}

	var b strings.Builder
	b.WriteString(`<script type="importmap">`)
	b.WriteString(strings.ReplaceAll(string(mapJson), "</", "<\\/"))
	b.WriteString("</script>\n")

	hasApplication := false
	for _, e := range entries {
		if e.Preload {
			_, _ = fmt.Fprintf(&b, "<link rel=\"modulepreload\" href=\"%s\">\n", template.HTMLEscapeString(e.URL))
		}
		if e.Name == "application" {
			hasApplication = true
		}
	}

	if hasApplication {
		b.WriteString(`<script type="module">import "application"</script>` + "\n")
	}

	return template.HTML(b.String()), nil
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type JspmResolver struct {
	httpClient httpDoer
}

type jspmGenerateRequest struct {
	Install      []string `json:"install"`
	FlattenScope bool     `json:"flattenScope"`
	Env          []string `json:"env"`
}

type jspmGenerateResponse struct {
	Map struct {
		Imports map[string]string `json:"imports"`
	} `json:"map"`
	Error string `json:"error"`
}

func (j *JspmResolver) client() httpDoer {
	if j.httpClient != nil {
		return j.httpClient
	}

	return http.DefaultClient
}

func (j *JspmResolver) Generate(pkg string) (map[string]string, error) {
	reqBody, err := json.Marshal(jspmGenerateRequest{
		Install:      []string{pkg},
		FlattenScope: true,
		Env:          []string{"browser", "module", "production"},
	})
	if err != nil {
		return nil, fmt.Errorf("assets: encoding jspm request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.jspm.io/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("assets: building jspm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("assets: calling jspm generate api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("assets: reading jspm response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("assets: jspm generate api returned %s: %s", resp.Status, string(body))
	}

	var out jspmGenerateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("assets: parsing jspm response: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("assets: jspm generate api error: %s", out.Error)
	}
	if len(out.Map.Imports) == 0 {
		return nil, fmt.Errorf("assets: jspm generate api returned no imports for %q", pkg)
	}

	return out.Map.Imports, nil
}

func Download(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("assets: downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("assets: downloading: %s: unexpected status %s", url, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("assets: reading download body for %s: %w", url, err)
	}

	return data, nil
}

var sourceMappingURLLine = regexp.MustCompile(`(?m)^[ \t]*//#\s*sourceMappingURL=\S+[ \t]*\n?`)

func StripSourceMappingURL(data []byte) []byte {
	return sourceMappingURLLine.ReplaceAll(data, nil)
}

func VendorFilename(specifier, url string) string {
	ext := ".js"
	if u, err := neturl.Parse(url); err == nil {
		if base := path.Base(u.Path); base != "" && base != "." && base != "/" {
			if e := path.Ext(base); e != "" {
				ext = e
			}
		}
	}

	replacer := strings.NewReplacer("@", "", "/", "-")

	return replacer.Replace(specifier) + ext
}
