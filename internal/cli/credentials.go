package cli

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/asimmons91/trails/internal/credentials"
)

type CredentialsCmd struct {
	Edit CredentialsEditCmd `cmd:"" help:"Decrypt, edit in $EDITOR, and re-encrypt credentials."`
	Show CredentialsShowCmd `cmd:"" help:"Print decrypted credentials to stdout."`
}

type CredentialsEditCmd struct {
	Environment string `help:"Environment to edit credentials for." short:"e" default:"development"`
}

func (c *CredentialsEditCmd) Run() error {
	logger := slog.Default()
	env := c.Environment

	keyPath := credentials.KeyPath(env)
	encPath := credentials.EncPath(env)

	encFileExists, err := fileExists(encPath)
	if err != nil {
		return fmt.Errorf("credentials: checking %s: %w", encPath, err)
	}

	key, err := credentials.ResolveKey(env)
	if err != nil {
		if encFileExists {
			return fmt.Errorf("credentials: %s exists but its master key could not be resolved: %w", encPath, err)
		}

		key, err = credentials.GenerateKey()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
			return fmt.Errorf("credentials: creating %s: %w", filepath.Dir(keyPath), err)
		}
		if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
			return fmt.Errorf("credentials: writing %s: %w", keyPath, err)
		}
		logger.Warn("credentials: generated a new master key; it is gitignored and not recoverable if lost, store it securely", "path", keyPath)
	}

	var plaintext []byte
	if encFileExists {
		data, err := os.ReadFile(encPath)
		if err != nil {
			return fmt.Errorf("credentials: reading %s: %w", encPath, err)
		}

		plaintext, err = credentials.Decrypt(key, string(data))
		if err != nil {
			return err
		}
	} else {
		secretKeyBase, err := credentials.GenerateKey()
		if err != nil {
			return err
		}
		plaintext = credentials.DefaultContents(secretKeyBase)
	}

	edited, err := editInEditor(plaintext)
	if err != nil {
		return err
	}

	if bytes.Equal(edited, plaintext) {
		logger.Info("credentials: no changes made")
		return nil
	}

	var check map[string]any
	if err := toml.Unmarshal(edited, &check); err != nil {
		return fmt.Errorf("credentials: not saving, invalid TOML: %w", err)
	}

	encoded, err := credentials.Encrypt(key, edited)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(encPath), 0o755); err != nil {
		return fmt.Errorf("credentials: creating %s: %w", filepath.Dir(encPath), err)
	}
	if err := os.WriteFile(encPath, []byte(encoded), 0o644); err != nil {
		return fmt.Errorf("credentials: writing %s: %w", encPath, err)
	}

	logger.Info("credentials updated", "path", encPath)
	return nil
}

type CredentialsShowCmd struct {
	Environment string `help:"Environment to show credentials for." short:"e" default:"development"`
}

func (c *CredentialsShowCmd) Run() error {
	env := c.Environment
	encPath := credentials.EncPath(env)

	data, err := os.ReadFile(encPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("credentials: no credentials configured for environment %q", env)
		}
		return fmt.Errorf("credentials: reading %s: %w", encPath, err)
	}

	key, err := credentials.ResolveKey(env)
	if err != nil {
		return err
	}

	plaintext, err := credentials.Decrypt(key, string(data))
	if err != nil {
		return err
	}

	fmt.Print(string(plaintext))
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// editInEditor writes content to a private temp file, opens it in $EDITOR
// (falling back to $VISUAL), waits for the editor to exit, and returns the
// file's contents afterward.
func editInEditor(content []byte) ([]byte, error) {
	tmp, err := os.CreateTemp("", "trails-credentials-*.toml")
	if err != nil {
		return nil, fmt.Errorf("credentials: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("credentials: securing temp file: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("credentials: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("credentials: closing temp file: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return nil, errors.New("credentials: no editor set; set $EDITOR (or $VISUAL) to edit credentials")
	}

	parts := strings.Fields(editor)
	args := append(append([]string{}, parts[1:]...), tmpPath)

	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("credentials: editor exited with error: %w", err)
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("credentials: reading edited temp file: %w", err)
	}

	return edited, nil
}
