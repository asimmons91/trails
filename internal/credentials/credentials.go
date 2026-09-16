package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const dir = "config/credentials"

const masterKeyEnvVar = "TRAILS_MASTER_KEY"

func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("credentials: generating master key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

func Encrypt(hexKey string, plaintext []byte) (string, error) {
	gcm, err := newGCM(hexKey)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("credentials: generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return base64.StdEncoding.EncodeToString(ciphertext) + "\n", nil
}

func Decrypt(hexKey, encoded string) ([]byte, error) {
	gcm, err := newGCM(hexKey)
	if err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("credentials: decoding ciphertext: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return nil, errors.New("credentials: ciphertext too short")
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("credentials: decrypting (wrong key or corrupted file): %w", err)
	}

	return plaintext, nil
}

func newGCM(hexKey string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("credentials: decoding master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("credentials: master key must be 32 bytes (64 hex chars), got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("credentials: creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credentials: creating GCM: %w", err)
	}

	return gcm, nil
}

func KeyPath(environment string) string {
	return filepath.Join(dir, environment+".key")
}

func EncPath(environment string) string {
	return filepath.Join(dir, environment+".toml.enc")
}

func RelEncPath(environment string) string {
	return path.Join("credentials", environment+".toml.enc")
}

func ResolveKey(environment string) (string, error) {
	if v := os.Getenv(masterKeyEnvVar); v != "" {
		return v, nil
	}

	path := KeyPath(environment)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("credentials: no master key found for %q: set %s or create %s", environment, masterKeyEnvVar, path)
		}
		return "", fmt.Errorf("credentials: reading %s: %w", path, err)
	}

	return strings.TrimSpace(string(data)), nil
}

func ReadDecrypted(fsys fs.FS, environment string) (plaintext []byte, ok bool, err error) {
	relPath := RelEncPath(environment)

	data, err := fs.ReadFile(fsys, relPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("credentials: reading %s: %w", relPath, err)
	}

	key, err := ResolveKey(environment)
	if err != nil {
		return nil, false, err
	}

	plaintext, err = Decrypt(key, string(data))
	if err != nil {
		return nil, false, err
	}

	return plaintext, true, nil
}

func DefaultContents() []byte {
	return []byte("# Add secrets here, e.g.:\n# secret_key_base = \"...\"\n")
}
