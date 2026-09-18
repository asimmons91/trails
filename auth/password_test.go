package auth_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/auth"
	"github.com/stretchr/testify/require"
)

func init() {
	// Speed up bcrypt for the whole test run.
	auth.BCryptCost = 4
}

func TestSecurePasswordBeforeInsertHashesPassword(t *testing.T) {
	p := &auth.SecurePassword{Password: "hunter2", PasswordConfirmation: "hunter2"}

	require.NoError(t, p.BeforeInsert(context.Background()))

	require.NotEmpty(t, p.PasswordDigest)
	require.NotEqual(t, "hunter2", p.PasswordDigest)
	require.Empty(t, p.Password)
	require.Empty(t, p.PasswordConfirmation)
	require.True(t, p.Authenticate("hunter2"))
	require.False(t, p.Authenticate("wrong"))
}

func TestSecurePasswordBeforeInsertRequiresPassword(t *testing.T) {
	p := &auth.SecurePassword{}

	err := p.BeforeInsert(context.Background())

	require.ErrorIs(t, err, auth.ErrPasswordRequired)
}

func TestSecurePasswordBeforeInsertAllowsPrePopulatedDigest(t *testing.T) {
	p := &auth.SecurePassword{PasswordDigest: "already-hashed-elsewhere"}

	require.NoError(t, p.BeforeInsert(context.Background()))
	require.Equal(t, "already-hashed-elsewhere", p.PasswordDigest)
}

func TestSecurePasswordBeforeInsertRejectsMismatchedConfirmation(t *testing.T) {
	p := &auth.SecurePassword{Password: "hunter2", PasswordConfirmation: "hunter3"}

	err := p.BeforeInsert(context.Background())

	require.ErrorIs(t, err, auth.ErrPasswordConfirmationMismatch)
}

func TestSecurePasswordConfirmationOptional(t *testing.T) {
	p := &auth.SecurePassword{Password: "hunter2"}

	require.NoError(t, p.BeforeInsert(context.Background()))
	require.True(t, p.Authenticate("hunter2"))
}

func TestSecurePasswordBeforeUpdateLeavesDigestUntouchedWhenBlank(t *testing.T) {
	p := &auth.SecurePassword{Password: "hunter2"}
	require.NoError(t, p.BeforeInsert(context.Background()))

	require.NoError(t, p.BeforeUpdate(context.Background()))

	require.True(t, p.Authenticate("hunter2"))
}

func TestSecurePasswordBeforeUpdateRehashesWhenPasswordSet(t *testing.T) {
	p := &auth.SecurePassword{Password: "hunter2"}
	require.NoError(t, p.BeforeInsert(context.Background()))

	p.Password = "newpassword"
	require.NoError(t, p.BeforeUpdate(context.Background()))

	require.False(t, p.Authenticate("hunter2"))
	require.True(t, p.Authenticate("newpassword"))
}

func TestSecurePasswordAuthenticateFalseWithNoDigest(t *testing.T) {
	p := &auth.SecurePassword{}

	require.False(t, p.Authenticate("anything"))
}
