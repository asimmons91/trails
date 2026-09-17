package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/internal/credentials"
)

// fakeEditor points $EDITOR at a tiny script that replaces the file it's
// given with content, simulating a user editing and saving.
func fakeEditor(t *testing.T, content string) {
	t.Helper()

	script := filepath.Join(t.TempDir(), "fake-editor.sh")
	body := "#!/bin/sh\ncat > \"$1\" <<'TRAILS_EOF'\n" + content + "\nTRAILS_EOF\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))

	t.Setenv("EDITOR", script)
}

func TestCredentialsEditCmdRunCreatesKeyAndEncryptedFileOnFreshInit(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")
	fakeEditor(t, `secret_key_base = "abc123"`)

	cmd := CredentialsEditCmd{Environment: "development"}
	require.NoError(t, cmd.Run())

	keyData, err := os.ReadFile(credentials.KeyPath("development"))
	require.NoError(t, err)
	require.Len(t, string(keyData), 65) // 64 hex chars + trailing newline

	encData, err := os.ReadFile(credentials.EncPath("development"))
	require.NoError(t, err)

	key, err := credentials.ResolveKey("development")
	require.NoError(t, err)
	plaintext, err := credentials.Decrypt(key, string(encData))
	require.NoError(t, err)
	require.Contains(t, string(plaintext), `secret_key_base = "abc123"`)
}

func TestCredentialsEditCmdRunReEncryptsExistingFileWithExistingKey(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	key, err := credentials.GenerateKey()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(credentials.KeyPath("development")), 0o755))
	require.NoError(t, os.WriteFile(credentials.KeyPath("development"), []byte(key+"\n"), 0o600))

	encoded, err := credentials.Encrypt(key, []byte("secret_key_base = \"old\"\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(credentials.EncPath("development"), []byte(encoded), 0o644))

	fakeEditor(t, `secret_key_base = "new"`)

	cmd := CredentialsEditCmd{Environment: "development"}
	require.NoError(t, cmd.Run())

	// Key must be unchanged.
	keyData, err := os.ReadFile(credentials.KeyPath("development"))
	require.NoError(t, err)
	require.Equal(t, key+"\n", string(keyData))

	encData, err := os.ReadFile(credentials.EncPath("development"))
	require.NoError(t, err)
	plaintext, err := credentials.Decrypt(key, string(encData))
	require.NoError(t, err)
	require.Contains(t, string(plaintext), `secret_key_base = "new"`)
}

func TestCredentialsEditCmdRunErrorsWhenEncFileExistsButKeyMissing(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	key, err := credentials.GenerateKey()
	require.NoError(t, err)
	encoded, err := credentials.Encrypt(key, []byte("secret_key_base = \"old\"\n"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(credentials.EncPath("development")), 0o755))
	require.NoError(t, os.WriteFile(credentials.EncPath("development"), []byte(encoded), 0o644))

	cmd := CredentialsEditCmd{Environment: "development"}
	err = cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "master key could not be resolved")

	_, statErr := os.Stat(credentials.KeyPath("development"))
	require.True(t, os.IsNotExist(statErr), "must not silently generate a new key over existing ciphertext")
}

func TestCredentialsEditCmdRunAbortsOnInvalidTOML(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")
	fakeEditor(t, "this is not valid [[[toml")

	cmd := CredentialsEditCmd{Environment: "development"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid TOML")

	_, statErr := os.Stat(credentials.EncPath("development"))
	require.True(t, os.IsNotExist(statErr))
}

func TestCredentialsEditCmdRunErrorsWithoutEditorSet(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	cmd := CredentialsEditCmd{Environment: "development"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no editor set")
}

func TestCredentialsShowCmdRunReturnsErrorWhenNoCredentialsConfigured(t *testing.T) {
	chdirTemp(t)

	cmd := CredentialsShowCmd{Environment: "development"}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), `no credentials configured for environment "development"`)
}

func TestCredentialsShowCmdRunPrintsDecryptedContent(t *testing.T) {
	chdirTemp(t)

	key, err := credentials.GenerateKey()
	require.NoError(t, err)
	t.Setenv("TRAILS_MASTER_KEY", key)

	encoded, err := credentials.Encrypt(key, []byte("secret_key_base = \"abc123\"\n"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(credentials.EncPath("development")), 0o755))
	require.NoError(t, os.WriteFile(credentials.EncPath("development"), []byte(encoded), 0o644))

	cmd := CredentialsShowCmd{Environment: "development"}
	var runErr error
	out := captureStdout(t, func() {
		runErr = cmd.Run()
	})
	require.NoError(t, runErr)
	require.Equal(t, "secret_key_base = \"abc123\"\n", out)
}
