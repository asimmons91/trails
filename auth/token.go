package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// tokenBytes is default 24 random bytes to ensure collision resistance.
const tokenBytes = 24

// NewToken returns a random, URL-safe opaque token suitable for
// has_secure_token-style columns (API keys, auth tokens, etc.) — 24 random
// bytes, base64.RawURLEncoding.
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generating token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// EnsureToken sets *field to a fresh NewToken() if it is currently empty.
// Call it from a model's BeforeInsert hook for each has_secure_token-style
// column:
//
//	func (u *User) BeforeInsert(ctx context.Context) error {
//	    return auth.EnsureToken(&u.AuthToken)
//	}
//
// To regenerate an existing token (Rails' regenerate_<token>!), just assign
// a fresh NewToken() and persist with pack.Update — no helper needed here.
func EnsureToken(field *string) error {
	if *field != "" {
		return nil
	}

	token, err := NewToken()
	if err != nil {
		return err
	}

	*field = token
	return nil
}
