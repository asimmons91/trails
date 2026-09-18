package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/auth"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
)

func init() {
	auth.BCryptCost = 4 // speed up bcrypt for tests
}

type authUser struct {
	pack.Model[int64] `db:"table:auth_users"`
	Email             string `db:"email,unique,not_null"`
	AuthToken         string `db:"auth_token,unique,not_null"`
	auth.SecurePassword
}

// BeforeInsert delegates to SecurePassword's hook (Go has no callback
// chaining) and additionally has_secure_token-generates an AuthToken.
func (u *authUser) BeforeInsert(ctx context.Context) error {
	if err := u.SecurePassword.BeforeInsert(ctx); err != nil {
		return err
	}
	return auth.EnsureToken(&u.AuthToken)
}

func newAuthTestDB(t *testing.T) *pack.DB {
	t.Helper()

	sqlDB, err := sqlite.New().Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := pack.Open(sqlDB, sqlite.New().Dialect())
	require.NoError(t, migrate.New(db).CreateTable(context.Background(), &authUser{}))

	return db
}

const testSecretKeyBase = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestSecurePasswordAndSecureTokenThroughPackCreate(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t)

	u := &authUser{Email: "a@b.com"}
	u.Password = "hunter2"
	u.PasswordConfirmation = "hunter2"

	require.NoError(t, pack.Create(ctx, db, u))

	require.NotZero(t, u.ID)
	require.NotEmpty(t, u.PasswordDigest)
	require.Empty(t, u.Password)
	require.NotEmpty(t, u.AuthToken)
	require.True(t, u.Authenticate("hunter2"))

	fetched, err := pack.ByID[authUser](ctx, db, u.ID)
	require.NoError(t, err)
	require.True(t, fetched.Authenticate("hunter2"))
	require.Equal(t, u.AuthToken, fetched.AuthToken)
}

func TestGenerateTokenForInvalidatedByPasswordChangeThroughPackUpdate(t *testing.T) {
	ctx := context.Background()
	db := newAuthTestDB(t)

	u := &authUser{Email: "a@b.com"}
	u.Password = "hunter2"
	require.NoError(t, pack.Create(ctx, db, u))

	token, err := auth.GenerateTokenFor(testSecretKeyBase, "password_reset",
		"42", u.PasswordDigest, time.Hour)
	require.NoError(t, err)

	id, ok := auth.VerifyTokenFor(testSecretKeyBase, "password_reset", token, u.PasswordDigest)
	require.True(t, ok)
	require.Equal(t, "42", id)

	u.Password = "newpassword"
	require.NoError(t, pack.Update(ctx, db, u))

	_, ok = auth.VerifyTokenFor(testSecretKeyBase, "password_reset", token, u.PasswordDigest)
	require.False(t, ok, "reset token must stop verifying once the password digest it was bound to changes")
}
