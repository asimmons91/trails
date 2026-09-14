package trails

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func newSettingsFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for path, content := range files {
		fsys[path] = &fstest.MapFile{Data: []byte(content)}
	}

	return fsys
}

func TestEnvironmentReturnsTrailsEnvValueWhenSet(t *testing.T) {
	t.Setenv("TRAILS_ENV", "staging")

	require.Equal(t, "staging", Environment())
}

func TestEnvironmentDefaultsToTestUnderGoTest(t *testing.T) {
	t.Setenv("TRAILS_ENV", "")

	require.Equal(t, "test", Environment())
}

type testAppConfig struct {
	Name   string `toml:"name"`
	Debug  bool   `toml:"debug"`
	Server struct {
		Host string `toml:"host"`
		Port int    `toml:"port"`
	} `toml:"server"`
}

const testAppConfigBaseTOML = `
name = "trails-app"
debug = false

[server]
host = "localhost"
port = 8080
`

const testAppConfigStagingOverlayTOML = `
[server]
port = 9090
`

func TestLoadMergesBaseAndEnvironmentConfig(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml":          testAppConfigBaseTOML,
		"environments/staging.toml": testAppConfigStagingOverlayTOML,
	})

	cfg, err := LoadConfig[testAppConfig]("staging", fsys)
	require.NoError(t, err)
	require.Equal(t, "trails-app", cfg.Name)
	require.False(t, cfg.Debug)
	require.Equal(t, "localhost", cfg.Server.Host)
	require.Equal(t, 9090, cfg.Server.Port)
}

func TestLoadMissingApplicationTomlReturnsWrappedError(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"environments/staging.toml": testAppConfigStagingOverlayTOML,
	})

	_, err := LoadConfig[testAppConfig]("staging", fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config/application.toml")
}

func TestLoadMissingEnvironmentTomlReturnsWrappedError(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml": testAppConfigBaseTOML,
	})

	_, err := LoadConfig[testAppConfig]("staging", fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config/environments/staging.toml")
}

func TestLoadMalformedApplicationTomlReturnsError(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml": `name = "trails-app`,
	})

	_, err := LoadConfig[testAppConfig]("staging", fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config/application.toml")
}

func TestLoadMalformedEnvironmentTomlReturnsError(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml":          testAppConfigBaseTOML,
		"environments/staging.toml": `[server`,
	})

	_, err := LoadConfig[testAppConfig]("staging", fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config/environments/staging.toml")
}

func TestLoadAppliesTrailsEnvOverridesOnTopOfFileValues(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml":          testAppConfigBaseTOML,
		"environments/staging.toml": testAppConfigStagingOverlayTOML,
	})
	t.Setenv("TRAILS_SERVER_PORT", "1234")

	cfg, err := LoadConfig[testAppConfig]("staging", fsys)
	require.NoError(t, err)
	require.Equal(t, 1234, cfg.Server.Port)
}

func TestLoadReturnsWrappedErrorForInvalidEnvOverride(t *testing.T) {
	fsys := newSettingsFS(map[string]string{
		"application.toml":          testAppConfigBaseTOML,
		"environments/staging.toml": testAppConfigStagingOverlayTOML,
	})
	t.Setenv("TRAILS_SERVER_PORT", "not-an-int")

	_, err := LoadConfig[testAppConfig]("staging", fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading TRAILS_* env overrides")
	require.Contains(t, err.Error(), "TRAILS_SERVER_PORT")
}

func TestApplyEnvOverridesSetsMatchingFields(t *testing.T) {
	t.Setenv("TRAILS_NAME", "override-name")
	t.Setenv("TRAILS_SERVER_HOST", "0.0.0.0")

	cfg := &testAppConfig{}
	require.NoError(t, applyEnvOverrides(cfg))
	require.Equal(t, "override-name", cfg.Name)
	require.Equal(t, "0.0.0.0", cfg.Server.Host)
}

func TestApplyEnvOverridesIgnoresUnmatchedTrailsVar(t *testing.T) {
	t.Setenv("TRAILS_NOPE_NOPE", "x")

	cfg := &testAppConfig{}
	require.NoError(t, applyEnvOverrides(cfg))
	require.Equal(t, testAppConfig{}, *cfg)
}

func TestApplyEnvOverridesWrapsResolveFieldError(t *testing.T) {
	t.Setenv("TRAILS_SERVER_PORT", "not-an-int")

	cfg := &testAppConfig{}
	err := applyEnvOverrides(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "setting TRAILS_SERVER_PORT")
}

type overrideScalars struct {
	Name     string   `toml:"name"`
	Port     int      `toml:"port"`
	Enabled  bool     `toml:"enabled"`
	Ratio    float64  `toml:"ratio"`
	Tags     []string `toml:"tags"`
	Untagged string
}

func TestResolveFieldScalarCoercion(t *testing.T) {
	tests := []struct {
		name       string
		fieldNames []string
		val        string
		wantMatch  bool
		wantErr    bool
		wantErrSub string
		assert     func(t *testing.T, tgt *overrideScalars)
	}{
		{
			name:       "string field matches",
			fieldNames: []string{"name"},
			val:        "trails-app",
			wantMatch:  true,
			assert: func(t *testing.T, tgt *overrideScalars) {
				require.Equal(t, "trails-app", tgt.Name)
			},
		},
		{
			name:       "int field matches and coerces",
			fieldNames: []string{"port"},
			val:        "8080",
			wantMatch:  true,
			assert: func(t *testing.T, tgt *overrideScalars) {
				require.Equal(t, 8080, tgt.Port)
			},
		},
		{
			name:       "bool field matches and coerces",
			fieldNames: []string{"enabled"},
			val:        "true",
			wantMatch:  true,
			assert: func(t *testing.T, tgt *overrideScalars) {
				require.True(t, tgt.Enabled)
			},
		},
		{
			name:       "float field matches and coerces",
			fieldNames: []string{"ratio"},
			val:        "0.5",
			wantMatch:  true,
			assert: func(t *testing.T, tgt *overrideScalars) {
				require.InDelta(t, 0.5, tgt.Ratio, 0)
			},
		},
		{
			name:       "invalid int value errors",
			fieldNames: []string{"port"},
			val:        "not-a-number",
			wantErr:    true,
		},
		{
			name:       "invalid bool value errors",
			fieldNames: []string{"enabled"},
			val:        "not-a-bool",
			wantErr:    true,
		},
		{
			name:       "invalid float value errors",
			fieldNames: []string{"ratio"},
			val:        "not-a-float",
			wantErr:    true,
		},
		{
			name:       "unsupported field kind errors",
			fieldNames: []string{"tags"},
			val:        "a",
			wantErr:    true,
			wantErrSub: "unsupported field kind",
		},
		{
			name:       "no tag matches the given name",
			fieldNames: []string{"missing"},
			val:        "x",
			wantMatch:  false,
		},
		{
			name:       "untagged field is never a candidate",
			fieldNames: []string{"untagged"},
			val:        "x",
			wantMatch:  false,
			assert: func(t *testing.T, tgt *overrideScalars) {
				require.Equal(t, "", tgt.Untagged)
			},
		},
		{
			name:       "empty names slice is a no-op",
			fieldNames: []string{},
			val:        "x",
			wantMatch:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tgt := &overrideScalars{}
			matched, err := resolveField(reflect.ValueOf(tgt).Elem(), tc.fieldNames, tc.val)

			require.Equal(t, tc.wantMatch, matched)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrSub != "" {
					require.Contains(t, err.Error(), tc.wantErrSub)
				}
			} else {
				require.NoError(t, err)
			}

			if tc.assert != nil {
				tc.assert(t, tgt)
			}
		})
	}
}

func TestResolveFieldReturnsFalseWhenValueIsNotStruct(t *testing.T) {
	n := 0
	matched, err := resolveField(reflect.ValueOf(&n).Elem(), []string{"x"}, "1")

	require.NoError(t, err)
	require.False(t, matched)
}

func TestResolveFieldRecursesIntoNestedStruct(t *testing.T) {
	type nestedLevel struct {
		Level string `toml:"level"`
	}
	type withNested struct {
		Log nestedLevel `toml:"log"`
	}

	tgt := &withNested{}
	matched, err := resolveField(reflect.ValueOf(tgt).Elem(), []string{"log", "level"}, "debug")

	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, "debug", tgt.Log.Level)
}

func TestResolveFieldPrefersLongerMatchAndFallsBackOnRecursionFailure(t *testing.T) {
	type deadEndLeaf struct {
		Extra string `toml:"extra"`
	}
	type matchingLeaf struct {
		LevelName string `toml:"level_name"`
	}
	type ambiguous struct {
		LogLevel deadEndLeaf  `toml:"log_level"`
		Log      matchingLeaf `toml:"log"`
	}

	tgt := &ambiguous{}
	matched, err := resolveField(reflect.ValueOf(tgt).Elem(), []string{"log", "level", "name"}, "verbose")

	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, "verbose", tgt.Log.LevelName)
	require.Empty(t, tgt.LogLevel.Extra)
}
