package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/asimmons91/trails/internal/credentials"
)

// tokenPayload is the plaintext embedded in a GenerateTokenFor token before
// encryption. Purpose and Digest are both checked on verify: Purpose keeps a
// token minted for one use from being replayed as another even though both
// share secretKeyBase; Digest lets the caller bind the token to some
// record state (e.g. a password hash) so it stops verifying the moment that
// state changes.
type tokenPayload struct {
	ID        string    `json:"id"`
	Purpose   string    `json:"purpose"`
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GenerateTokenFor returns a signed, expiring, purpose-scoped token for id —
// a generates_token_for equivalent for flows like password resets or email
// confirmation:
//
//	token, _ := auth.GenerateTokenFor(secretKeyBase, "password_reset",
//	    strconv.FormatInt(user.ID, 10), user.PasswordDigest, 15*time.Minute)
//	// emailed as a link: /reset_password?token=...
//
// The token is encrypted (not just signed) using the same
// internal/credentials AES-256-GCM primitive session.Middleware uses for
// session cookies, keyed by the same secretKeyBase — so id, purpose, and
// digest are never readable from the token itself, and any tampering makes
// it fail to decrypt. digest should be a value that changes when the token
// should be invalidated (e.g. the record's password digest); pass "" if
// there's nothing to bind it to.
func GenerateTokenFor(secretKeyBase, purpose, id, digest string, expiresIn time.Duration) (string, error) {
	payload := tokenPayload{
		ID:        id,
		Purpose:   purpose,
		Digest:    digest,
		ExpiresAt: time.Now().Add(expiresIn),
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	encoded, err := credentials.Encrypt(secretKeyBase, plaintext)
	if err != nil {
		return "", err
	}

	// credentials.Encrypt returns standard base64 (with '+'/'/') plus a
	// trailing newline; re-encode as URL-safe so the token drops straight
	// into a query param with no escaping.
	raw, err := base64.StdEncoding.DecodeString(trimTrailingNewline(encoded))
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// VerifyTokenFor verifies a token produced by GenerateTokenFor for purpose,
// checking it decrypts, has not expired, and was minted with the same
// digest. It returns the token's id and true only if every check passes —
// like Rails' find_by_token_for, no distinction is made between an
// expired, tampered, wrong-purpose, or digest-mismatched token, so a
// caller can't use timing or error content to probe which one it was.
func VerifyTokenFor(secretKeyBase, purpose, token, digest string) (id string, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}

	plaintext, err := credentials.Decrypt(secretKeyBase, base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		return "", false
	}

	var payload tokenPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return "", false
	}

	if payload.Purpose != purpose {
		return "", false
	}

	if time.Now().After(payload.ExpiresAt) {
		return "", false
	}

	if subtle.ConstantTimeCompare([]byte(payload.Digest), []byte(digest)) != 1 {
		return "", false
	}

	return payload.ID, true
}

func trimTrailingNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}
