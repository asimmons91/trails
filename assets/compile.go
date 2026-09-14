package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var allowedExt = map[string]bool{
	".css": true,
	".js":  true,
	".map": true,
}

type refPattern struct {
	regex   *regexp.Regexp
	rewrite func(digestedURL string) string
}

var refPatterns = map[string]refPattern{
	".css": {
		regex: regexp.MustCompile(`@import\s+url\(\s*['"]?([^'")]+)['"]?\s*\)`),
		rewrite: func(digestedURL string) string {
			return fmt.Sprintf(`@import url("%s")`, digestedURL)
		},
	},
	".js": {
		regex: regexp.MustCompile(`//#\s*sourceMappingURL=(\S+)`),
		rewrite: func(digestedURL string) string {
			return "//# sourceMappingURL=" + digestedURL
		},
	},
}

func Compile(sourceDir, outputDir, prefix string) (Manifest, error) {
	if err := os.RemoveAll(outputDir); err != nil {
		return nil, fmt.Errorf("assets: cleaning output dir: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("assets: creating output dir: %w", err)
	}

	files, err := readSourceFiles(sourceDir)
	if err != nil {
		return nil, err
	}

	refs, err := resolveReferences(files)
	if err != nil {
		return nil, err
	}

	manifest := Manifest{}
	for relPath := range files {
		manifest[relPath] = digestedName(relPath, digest(files, refs, relPath))
	}

	for relPath, content := range files {
		out := content
		if _, ok := refPatterns[path.Ext(relPath)]; ok {
			out = []byte(rewriteReferences(string(content), relPath, manifest, prefix))
		}

		outPath := filepath.Join(outputDir, filepath.FromSlash(manifest[relPath]))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return nil, fmt.Errorf("assets: creating output subdir: %w", err)
		}
		if err := os.WriteFile(outPath, out, 0o644); err != nil {
			return nil, fmt.Errorf("assets: writing %s: %w", outPath, err)
		}
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("assets: encoding manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "manifest.json"), manifestBytes, 0o644); err != nil {
		return nil, fmt.Errorf("assets: writing manifest: %w", err)
	}

	return manifest, nil
}

func readSourceFiles(sourceDir string) (map[string][]byte, error) {
	files := map[string][]byte{}

	err := filepath.WalkDir(sourceDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !allowedExt[filepath.Ext(p)] {
			return nil
		}

		rel, err := filepath.Rel(sourceDir, p)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}

		files[filepath.ToSlash(rel)] = data

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assets: walking %s: %w", sourceDir, err)
	}

	return files, nil
}

func resolveReferences(files map[string][]byte) (map[string][]string, error) {
	refs := map[string][]string{}

	for relPath, content := range files {
		pattern, ok := refPatterns[path.Ext(relPath)]
		if !ok {
			continue
		}

		for _, m := range pattern.regex.FindAllStringSubmatch(string(content), -1) {
			target := m[1]
			if isExternal(target) {
				continue
			}

			resolved := resolveImportTarget(relPath, target)
			if _, ok := files[resolved]; !ok {
				return nil, fmt.Errorf("assets: %s references unknown local file %q", relPath, target)
			}

			refs[relPath] = append(refs[relPath], resolved)
		}
	}

	return refs, nil
}

func resolveImportTarget(relPath, target string) string {
	return path.Clean(path.Join(path.Dir(relPath), target))
}

func digest(files map[string][]byte, refs map[string][]string, relPath string) string {
	h := sha256.New()
	h.Write(files[relPath])

	visited := map[string]bool{relPath: true}
	var walk func(string)
	walk = func(p string) {
		refList := append([]string(nil), refs[p]...)
		sort.Strings(refList)
		for _, ref := range refList {
			if visited[ref] {
				continue
			}
			visited[ref] = true
			h.Write(files[ref])
			walk(ref)
		}
	}
	walk(relPath)

	return hex.EncodeToString(h.Sum(nil))[:16]
}

func digestedName(relPath, digest string) string {
	dir, file := path.Split(relPath)
	ext := path.Ext(file)
	base := strings.TrimSuffix(file, ext)

	return dir + base + "-" + digest + ext
}

func rewriteReferences(content, relPath string, manifest Manifest, prefix string) string {
	pattern, ok := refPatterns[path.Ext(relPath)]
	if !ok {
		return content
	}

	return pattern.regex.ReplaceAllStringFunc(content, func(match string) string {
		sub := pattern.regex.FindStringSubmatch(match)
		target := sub[1]
		if isExternal(target) {
			return match
		}

		resolved := resolveImportTarget(relPath, target)
		digested, ok := manifest.Path(resolved)
		if !ok {
			return match
		}

		return pattern.rewrite(path.Join(prefix, digested))
	})
}

func isExternal(target string) bool {
	return strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "//") ||
		strings.HasPrefix(target, "data:")
}
