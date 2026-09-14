package pack

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileFail(t *testing.T) {
	cases := []struct {
		name          string
		file          string
		wantSubstring string
	}{
		{
			name:          "wrong value type in a predicate",
			file:          "wrong_predicate_value_type.go",
			wantSubstring: `cannot use "eighteen"`,
		},
		{
			name:          "text predicate on a non-string column",
			file:          "text_predicate_on_non_string_column.go",
			wantSubstring: "does not match inferred type",
		},
		{
			name:          "wrong key type",
			file:          "wrong_key_type.go",
			wantSubstring: `cannot use "abc"`,
		},
		{
			name:          "key-based function on a key-less type",
			file:          "key_based_function_on_key_less_type.go",
			wantSubstring: "missing method PK",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", "compilefail", tc.file)
			cmd := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "out"), path)
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "expected %s to fail to compile; output:\n%s", path, out)
			assert.Contains(t, string(out), tc.wantSubstring)
		})
	}
}
