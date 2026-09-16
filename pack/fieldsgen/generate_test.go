package fieldsgen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// updateGolden regenerates the golden files this test compares against.
// Run with: UPDATE_GOLDEN=1 go test ./pack/fieldsgen/... -run TestGenerateGolden
var updateGolden = os.Getenv("UPDATE_GOLDEN") != ""

func TestGenerateGolden(t *testing.T) {
	cases := []string{"basic", "compositepk", "tablenameonly"}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", name)
			goldenPath := filepath.Join(dir, "expected.golden")

			res, err := Generate(Options{Dir: dir, OutFile: "trails_fields_gen.go"})
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Remove(res.OutPath) })

			got, err := os.ReadFile(res.OutPath)
			require.NoError(t, err)

			if updateGolden {
				require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file missing; run with UPDATE_GOLDEN=1 to create it")
			require.Equal(t, string(want), string(got), "generated output does not match %s", goldenPath)

			// The generated file must actually compile alongside its
			// source models, not just look plausible.
			cmd := exec.Command("go", "build", "./"+dir)
			out, err := cmd.CombinedOutput()
			require.NoErrorf(t, err, "generated output for %s does not compile:\n%s", dir, out)
		})
	}
}

// TestGenerateRerunIsStable verifies that regenerating over a file that
// already exists on disk works: a naive implementation would try to
// type-check the stale output (which references possibly-renamed fields)
// and deadlock. This is the highest-risk part of the design (see load.go's
// overlay handling).
func TestGenerateRerunIsStable(t *testing.T) {
	dir := filepath.Join("testdata", "basic")

	res1, err := Generate(Options{Dir: dir, OutFile: "trails_fields_gen.go"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(res1.OutPath) })

	first, err := os.ReadFile(res1.OutPath)
	require.NoError(t, err)

	res2, err := Generate(Options{Dir: dir, OutFile: "trails_fields_gen.go"})
	require.NoError(t, err)

	second, err := os.ReadFile(res2.OutPath)
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "regenerating should be idempotent")
}

func TestGenerateNoModels(t *testing.T) {
	_, err := Generate(Options{Dir: filepath.Join("testdata", "empty"), OutFile: "trails_fields_gen.go"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pack models found")
}
