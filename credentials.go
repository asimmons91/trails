package trails

import (
	"fmt"
	"io/fs"

	"github.com/pelletier/go-toml/v2"

	"github.com/asimmons91/trails/internal/credentials"
)

// LoadCredentials reads and decrypts environment's encrypted credentials
// file (see internal/credentials) from credentialsFS and TOML-unmarshals
// it into T. If the file doesn't exist, it returns a zero-value T and no
// error — only a wrong/missing decryption key is an error. See
// LoadConfig for loading it as one layer of a full app config instead of
// standalone.
func LoadCredentials[T any](environment string, credentialsFS fs.FS) (*T, error) {
	var out T

	plaintext, ok, err := credentials.ReadDecrypted(credentialsFS, environment)
	if err != nil {
		return nil, fmt.Errorf("loading credentials for %q: %w", environment, err)
	}
	if !ok {
		return &out, nil
	}

	if err := toml.Unmarshal(plaintext, &out); err != nil {
		return nil, fmt.Errorf("parsing credentials for %q: %w", environment, err)
	}

	return &out, nil
}
