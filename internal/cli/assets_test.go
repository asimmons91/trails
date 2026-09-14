package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCmdRunRunsScriptsThenFingerprintsOutput(t *testing.T) {
	chdirTemp(t)

	writeFile(t, "package.json", `{}`)
	writeFile(t, filepath.Join("frontend", "application.js"), `console.log("hi");`)

	cmd := BuildCmd{
		Source: "frontend",
		Output: "public/assets",
		Prefix: "/assets",
		Dir:    ".",
	}

	err := cmd.Run()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join("public/assets", "manifest.json"))
	require.NoError(t, err)

	var manifest map[string]string
	require.NoError(t, json.Unmarshal(data, &manifest))
	require.Contains(t, manifest, "application.js")
}

func TestBuildCmdRunReturnsErrorWhenScriptFails(t *testing.T) {
	chdirTemp(t)

	cmd := BuildCmd{
		Source:  "frontend",
		Output:  "public/assets",
		Prefix:  "/assets",
		Dir:     ".",
		Scripts: []string{"build"},
	}

	err := cmd.Run()
	require.Error(t, err)
}
