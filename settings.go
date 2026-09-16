package trails

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/asimmons91/trails/internal/credentials"
)

func Environment() string {
	if v := os.Getenv("TRAILS_ENV"); v != "" {
		return v
	}

	if testing.Testing() {
		return "test"
	}

	return "development"
}

func LoadConfig[T any](environment string, configFS fs.FS) (*T, error) {
	var cfg T

	if err := decodeTOML(configFS, "application.toml", &cfg); err != nil {
		return nil, fmt.Errorf("loading config/application.toml: %w", err)
	}

	envConfig := fmt.Sprintf("environments/%s.toml", environment)
	if err := decodeTOML(configFS, envConfig, &cfg); err != nil {
		return nil, fmt.Errorf("loading config/%s: %w", envConfig, err)
	}

	if err := decodeCredentials(environment, configFS, &cfg); err != nil {
		return nil, fmt.Errorf("loading credentials for %q: %w", environment, err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, fmt.Errorf("loading TRAILS_* env overrides: %w", err)
	}

	return &cfg, nil
}

func decodeTOML(configFS fs.FS, name string, v any) error {
	data, err := fs.ReadFile(configFS, name)
	if err != nil {
		return err
	}

	return toml.NewDecoder(bytes.NewReader(data)).Decode(v)
}

func decodeCredentials(environment string, configFS fs.FS, cfg any) error {
	plaintext, ok, err := credentials.ReadDecrypted(configFS, environment)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	return toml.Unmarshal(plaintext, cfg)
}

func applyEnvOverrides(cfg any) error {
	rv := reflect.ValueOf(cfg).Elem()

	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, "TRAILS_") {
			continue
		}

		names := strings.Split(strings.ToLower(strings.TrimPrefix(key, "TRAILS_")), "_")
		if _, err := resolveField(rv, names, val); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
	}

	return nil
}

func resolveField(v reflect.Value, names []string, val string) (bool, error) {
	if v.Kind() != reflect.Struct || len(names) == 0 {
		return false, nil
	}

	t := v.Type()

	type candidate struct {
		fieldIdx int
		consumed int
	}

	var candidates []candidate
	for i := range t.NumField() {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("toml"), ",")
		if tag == "" {
			continue
		}

		tagNames := strings.Split(tag, "_")
		if len(tagNames) > len(names) {
			continue
		}

		match := true
		for j, n := range tagNames {
			if names[j] != n {
				match = false
				break
			}
		}

		if match {
			candidates = append(candidates, candidate{i, len(tagNames)})
		}
	}

	sort.Slice(candidates, func(a, b int) bool {
		return candidates[a].consumed > candidates[b].consumed
	})

	for _, c := range candidates {
		fv := v.Field(c.fieldIdx)
		rest := names[c.consumed:]

		if len(rest) == 0 {
			if err := setScalar(fv, val); err != nil {
				return false, err
			}

			return true, nil
		}

		ok, err := resolveField(fv, rest, val)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}
