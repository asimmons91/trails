package auth_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/asimmons91/trails/auth"
	"github.com/asimmons91/trails/internal/credentials"
	"github.com/stretchr/testify/require"
)

func testSecretKeyBase(t *testing.T) string {
	t.Helper()

	key, err := credentials.GenerateKey()
	require.NoError(t, err)

	return key
}

func TestGenerateVerifyTokenForRoundTrip(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	id, ok := auth.VerifyTokenFor(key, "password_reset", token, "digest-v1")
	require.True(t, ok)
	require.Equal(t, "42", id)
}

func TestVerifyTokenForIsURLSafe(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", time.Hour)
	require.NoError(t, err)

	require.NotContains(t, token, "+")
	require.NotContains(t, token, "/")
	require.NotContains(t, token, "=")
	_, err = base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
}

func TestVerifyTokenForRejectsWrongPurpose(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", time.Hour)
	require.NoError(t, err)

	_, ok := auth.VerifyTokenFor(key, "email_confirmation", token, "digest-v1")
	require.False(t, ok)
}

func TestVerifyTokenForRejectsExpiredToken(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", -time.Minute)
	require.NoError(t, err)

	_, ok := auth.VerifyTokenFor(key, "password_reset", token, "digest-v1")
	require.False(t, ok)
}

func TestVerifyTokenForRejectsChangedDigest(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "old-digest", time.Hour)
	require.NoError(t, err)

	_, ok := auth.VerifyTokenFor(key, "password_reset", token, "new-digest")
	require.False(t, ok)
}

func TestVerifyTokenForRejectsTamperedToken(t *testing.T) {
	key := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", time.Hour)
	require.NoError(t, err)

	tampered := []byte(token)
	tampered[len(tampered)-1] ^= 0xFF

	_, ok := auth.VerifyTokenFor(key, "password_reset", string(tampered), "digest-v1")
	require.False(t, ok)
}

func TestVerifyTokenForRejectsWrongKey(t *testing.T) {
	key := testSecretKeyBase(t)
	otherKey := testSecretKeyBase(t)

	token, err := auth.GenerateTokenFor(key, "password_reset", "42", "digest-v1", time.Hour)
	require.NoError(t, err)

	_, ok := auth.VerifyTokenFor(otherKey, "password_reset", token, "digest-v1")
	require.False(t, ok)
}

func TestVerifyTokenForRejectsGarbageInput(t *testing.T) {
	key := testSecretKeyBase(t)

	_, ok := auth.VerifyTokenFor(key, "password_reset", "not-a-real-token", "digest-v1")
	require.False(t, ok)
}
