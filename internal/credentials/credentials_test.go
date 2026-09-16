package credentials

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func chdirTemp(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(orig))
	})
}

func TestGenerateKeyReturnsDistinct32ByteHexKeys(t *testing.T) {
	a, err := GenerateKey()
	require.NoError(t, err)
	require.Len(t, a, 64)

	b, err := GenerateKey()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	encoded, err := Encrypt(key, []byte("secret_key_base = \"abc\"\n"))
	require.NoError(t, err)

	plaintext, err := Decrypt(key, encoded)
	require.NoError(t, err)
	require.Equal(t, "secret_key_base = \"abc\"\n", string(plaintext))
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)
	other, err := GenerateKey()
	require.NoError(t, err)

	encoded, err := Encrypt(key, []byte("x = 1"))
	require.NoError(t, err)

	_, err = Decrypt(other, encoded)
	require.Error(t, err)
}

func TestDecryptDetectsTamperedCiphertext(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	encoded, err := Encrypt(key, []byte("x = 1"))
	require.NoError(t, err)

	tampered := []byte(encoded)
	// Flip a byte in the middle of the base64 body (leave the trailing
	// newline alone) so it still decodes as valid base64 of the wrong length.
	tampered[len(tampered)/2] ^= 0xFF

	_, err = Decrypt(key, string(tampered))
	require.Error(t, err)
}

func TestEncryptRejectsMalformedKey(t *testing.T) {
	_, err := Encrypt("not-hex", []byte("x"))
	require.Error(t, err)

	_, err = Encrypt("abcd", []byte("x"))
	require.Error(t, err)
}

func TestResolveKeyPrefersEnvVar(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "envkey")

	key, err := ResolveKey("development")
	require.NoError(t, err)
	require.Equal(t, "envkey", key)
}

func TestResolveKeyFallsBackToKeyFile(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	require.NoError(t, os.MkdirAll(filepath.Dir(KeyPath("development")), 0o755))
	require.NoError(t, os.WriteFile(KeyPath("development"), []byte("filekey\n"), 0o600))

	key, err := ResolveKey("development")
	require.NoError(t, err)
	require.Equal(t, "filekey", key)
}

func TestResolveKeyErrorsWhenNeitherPresent(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	_, err := ResolveKey("development")
	require.Error(t, err)
	require.Contains(t, err.Error(), "TRAILS_MASTER_KEY")
}

func TestReadDecryptedReturnsNotOkWhenFileMissing(t *testing.T) {
	chdirTemp(t)

	fsys := fstest.MapFS{}
	plaintext, ok, err := ReadDecrypted(fsys, "development")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, plaintext)
}

func TestReadDecryptedReturnsErrorWhenKeyMissing(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	key, err := GenerateKey()
	require.NoError(t, err)
	encoded, err := Encrypt(key, []byte("x = 1"))
	require.NoError(t, err)

	fsys := fstest.MapFS{
		"credentials/development.toml.enc": &fstest.MapFile{Data: []byte(encoded)},
	}

	_, ok, err := ReadDecrypted(fsys, "development")
	require.Error(t, err)
	require.False(t, ok)
}

func TestReadDecryptedDecryptsWithEnvKey(t *testing.T) {
	chdirTemp(t)

	key, err := GenerateKey()
	require.NoError(t, err)
	t.Setenv("TRAILS_MASTER_KEY", key)

	encoded, err := Encrypt(key, []byte("secret_key_base = \"abc\"\n"))
	require.NoError(t, err)

	fsys := fstest.MapFS{
		"credentials/development.toml.enc": &fstest.MapFile{Data: []byte(encoded)},
	}

	plaintext, ok, err := ReadDecrypted(fsys, "development")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "secret_key_base = \"abc\"\n", string(plaintext))
}
