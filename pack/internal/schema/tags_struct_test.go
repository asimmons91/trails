package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStructTag_TableAndAlias_BothCaptured(t *testing.T) {
	opts, err := ParseStructTag("User", "table:users,alias:u")
	require.NoError(t, err)
	assert.Equal(t, "users", opts.Table)
	assert.True(t, opts.HasTable)
	assert.Equal(t, "u", opts.Alias)
	assert.True(t, opts.HasAlias)
}

func TestParseStructTag_TableAlone_AliasUnset(t *testing.T) {
	opts, err := ParseStructTag("User", "table:users")
	require.NoError(t, err)
	assert.Equal(t, "users", opts.Table)
	assert.False(t, opts.HasAlias)
}

func TestParseStructTag_Empty_NoOptions(t *testing.T) {
	opts, err := ParseStructTag("User", "")
	require.NoError(t, err)
	assert.Equal(t, StructOptions{}, opts)
}

func TestParseStructTag_FieldOption_IsError(t *testing.T) {
	for _, raw := range []string{"table:users,pk", "table:users,-", "table:users,null_zero"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseStructTag("User", raw)
			require.Error(t, err)
			var fErr *ErrFieldOptionOnStructTag
			require.True(t, errors.As(err, &fErr))
		})
	}
}

func TestParseStructTag_UnknownOption_IsError(t *testing.T) {
	_, err := ParseStructTag("User", "bogus")
	require.Error(t, err)
	var uErr *ErrUnknownOption
	require.True(t, errors.As(err, &uErr))
	assert.Equal(t, "bogus", uErr.Option)
	assert.Equal(t, "", uErr.Field)
}
