package trails

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/internal/credentials"
)

type testCredentials struct {
	SecretKeyBase string `toml:"secret_key_base"`
}

func TestLoadCredentialsDecodesDecryptedFile(t *testing.T) {
	key, err := credentials.GenerateKey()
	require.NoError(t, err)
	t.Setenv("TRAILS_MASTER_KEY", key)

	fsys := fstest.MapFS{
		"credentials/staging.toml.enc": &fstest.MapFile{
			Data: []byte(encryptedCredentials(t, key, `secret_key_base = "abc123"`)),
		},
	}

	creds, err := LoadCredentials[testCredentials]("staging", fsys)
	require.NoError(t, err)
	require.Equal(t, "abc123", creds.SecretKeyBase)
}

func TestLoadCredentialsReturnsZeroValueWhenFileMissing(t *testing.T) {
	creds, err := LoadCredentials[testCredentials]("staging", fstest.MapFS{})
	require.NoError(t, err)
	require.Equal(t, testCredentials{}, *creds)
}

func TestLoadCredentialsReturnsErrorWhenKeyMissing(t *testing.T) {
	chdirTemp(t)
	t.Setenv("TRAILS_MASTER_KEY", "")

	key, err := credentials.GenerateKey()
	require.NoError(t, err)

	fsys := fstest.MapFS{
		"credentials/staging.toml.enc": &fstest.MapFile{
			Data: []byte(encryptedCredentials(t, key, `secret_key_base = "abc123"`)),
		},
	}

	_, err = LoadCredentials[testCredentials]("staging", fsys)
	require.Error(t, err)
}
