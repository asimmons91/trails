package trails

import (
	"fmt"
	"io/fs"

	"github.com/pelletier/go-toml/v2"

	"github.com/asimmons91/trails/internal/credentials"
)

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
