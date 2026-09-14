package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFieldTag_Column_UntaggedField_UsesDefaultNameNoOptions(t *testing.T) {
	tag, err := ParseFieldTag("User", "Age", "")
	require.NoError(t, err)
	assert.Equal(t, KindColumn, tag.Kind)
	assert.Equal(t, "", tag.NameSlot)
	assert.False(t, tag.HasLeadingComma)
	assert.Equal(t, ColumnOptions{}, tag.Column)
}

func TestParseFieldTag_Column_LeadingComma_AppliesOptionWithDefaultName(t *testing.T) {
	tag, err := ParseFieldTag("User", "CreatedAt", ",not_null")
	require.NoError(t, err)
	assert.Equal(t, "", tag.NameSlot)
	assert.True(t, tag.HasLeadingComma)
	assert.True(t, tag.Column.NotNull)
}

func TestParseFieldTag_Column_ExplicitName_NoOptions(t *testing.T) {
	tag, err := ParseFieldTag("User", "Email", "email")
	require.NoError(t, err)
	assert.Equal(t, "email", tag.NameSlot)
	assert.False(t, tag.HasLeadingComma)
	assert.Equal(t, ColumnOptions{}, tag.Column)
}

func TestParseFieldTag_Column_ExplicitNamePlusOption(t *testing.T) {
	tag, err := ParseFieldTag("User", "LastSeen", "seen_at,null_zero")
	require.NoError(t, err)
	assert.Equal(t, "seen_at", tag.NameSlot)
	assert.True(t, tag.Column.NullZero)
}

func TestParseFieldTag_Column_DashSkipsField(t *testing.T) {
	tag, err := ParseFieldTag("User", "Scratch", "-")
	require.NoError(t, err)
	assert.True(t, tag.Column.Skip)
}

func TestParseFieldTag_Column_BareOptionInSlot_IsAmbiguousError(t *testing.T) {
	for _, opt := range []string{"pk", "auto_increment", "null_zero", "unique", "not_null"} {
		t.Run(opt, func(t *testing.T) {
			_, err := ParseFieldTag("User", "F", opt)
			require.Error(t, err)
			var ambErr *ErrAmbiguousColumnSlot
			require.True(t, errors.As(err, &ambErr), "expected ErrAmbiguousColumnSlot, got %T: %v", err, err)
			assert.Equal(t, "User", ambErr.Struct)
			assert.Equal(t, "F", ambErr.Field)
			assert.Equal(t, opt, ambErr.Element)
		})
	}
}

func TestParseFieldTag_Column_PrefixOptionInSlot_IsAmbiguousError(t *testing.T) {
	for _, opt := range []string{"default:now()", "type:jsonb", "fk:x", "ref:x"} {
		t.Run(opt, func(t *testing.T) {
			_, err := ParseFieldTag("User", "F", opt)
			require.Error(t, err)
			var ambErr *ErrAmbiguousColumnSlot
			require.True(t, errors.As(err, &ambErr), "expected ErrAmbiguousColumnSlot, got %T: %v", err, err)
			assert.Equal(t, opt, ambErr.Element)
		})
	}
}

func TestParseFieldTag_Column_TrailingComma_IsDeliberateEscape(t *testing.T) {
	tag, err := ParseFieldTag("User", "Notnull", "notnull,")
	require.NoError(t, err)
	assert.Equal(t, "notnull", tag.NameSlot)
	assert.Equal(t, ColumnOptions{}, tag.Column)
}

func TestParseFieldTag_Column_FKAndRefAsOption_IsWrongGrammarError(t *testing.T) {
	for _, raw := range []string{"email,fk:x", "email,ref:x"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseFieldTag("User", "F", raw)
			require.Error(t, err)
			var wgErr *ErrOptionWrongGrammar
			require.True(t, errors.As(err, &wgErr))
			assert.Equal(t, "column", wgErr.Expected)
		})
	}
}

func TestParseFieldTag_Column_UnknownOption_IsError(t *testing.T) {
	_, err := ParseFieldTag("User", "F", "f,bogus")
	require.Error(t, err)
	var uErr *ErrUnknownOption
	require.True(t, errors.As(err, &uErr))
	assert.Equal(t, "bogus", uErr.Option)
}

func TestParseFieldTag_Column_TableOrAliasAsOption_IsStructOptionOnFieldTagError(t *testing.T) {
	for _, raw := range []string{"email,table:users", "email,alias:u"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseFieldTag("User", "F", raw)
			require.Error(t, err)
			var sErr *ErrStructOptionOnFieldTag
			require.True(t, errors.As(err, &sErr))
		})
	}
}
