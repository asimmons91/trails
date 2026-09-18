package auth_test

import (
	"testing"

	"github.com/asimmons91/trails/auth"
	"github.com/stretchr/testify/require"
)

func TestNewTokenReturnsDistinctURLSafeTokens(t *testing.T) {
	a, err := auth.NewToken()
	require.NoError(t, err)
	require.NotEmpty(t, a)
	require.NotContains(t, a, "+")
	require.NotContains(t, a, "/")
	require.NotContains(t, a, "=")

	b, err := auth.NewToken()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestEnsureTokenSetsEmptyField(t *testing.T) {
	var field string

	require.NoError(t, auth.EnsureToken(&field))

	require.NotEmpty(t, field)
}

func TestEnsureTokenLeavesExistingValueAlone(t *testing.T) {
	field := "existing-token"

	require.NoError(t, auth.EnsureToken(&field))

	require.Equal(t, "existing-token", field)
}
