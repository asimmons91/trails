package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFieldTag_Relation_NoLeadingComma_ParsesCleanly(t *testing.T) {
	tag, err := ParseFieldTag("User", "Posts", "rel:has_many,fk:user_id")
	require.NoError(t, err)
	assert.Equal(t, KindRelation, tag.Kind)
	assert.Equal(t, HasMany, tag.Relation.Kind)
	assert.Equal(t, "user_id", tag.Relation.FK)
}

func TestParseFieldTag_Relation_EachKind(t *testing.T) {
	cases := []struct {
		raw  string
		kind RelationKind
	}{
		{"rel:belongs_to,fk:author_id", BelongsTo},
		{"rel:has_one,fk:user_id", HasOne},
		{"rel:has_many,fk:user_id", HasMany},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			tag, err := ParseFieldTag("User", "F", tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.kind, tag.Relation.Kind)
		})
	}
}

func TestParseFieldTag_Relation_UnknownKind_IsError(t *testing.T) {
	_, err := ParseFieldTag("User", "F", "rel:has_and_belongs_to_many")
	require.Error(t, err)
	var kErr *ErrUnknownRelationKind
	require.True(t, errors.As(err, &kErr))
	assert.Equal(t, "has_and_belongs_to_many", kErr.Kind)
}

func TestParseFieldTag_Relation_LeadingComma_IsMisplacedRelError(t *testing.T) {
	_, err := ParseFieldTag("User", "F", ",rel:has_many")
	require.Error(t, err)
	var mErr *ErrMisplacedRelPrefix
	require.True(t, errors.As(err, &mErr))
	assert.Equal(t, 1, mErr.Index)
}

func TestParseFieldTag_Relation_MisplacedAfterOption_IsMisplacedRelError(t *testing.T) {
	_, err := ParseFieldTag("User", "F", "fk:x,rel:has_many")
	require.Error(t, err)
	var mErr *ErrMisplacedRelPrefix
	require.True(t, errors.As(err, &mErr))
	assert.Equal(t, 1, mErr.Index)
}

func TestParseFieldTag_Relation_ColumnOptionOnRelationTag_IsWrongGrammarError(t *testing.T) {
	_, err := ParseFieldTag("User", "F", "rel:has_many,pk")
	require.Error(t, err)
	var wgErr *ErrOptionWrongGrammar
	require.True(t, errors.As(err, &wgErr))
	assert.Equal(t, "relation", wgErr.Expected)
}

func TestParseFieldTag_Relation_RefGivenAndOmitted(t *testing.T) {
	withRef, err := ParseFieldTag("User", "F", "rel:belongs_to,fk:author_id,ref:id")
	require.NoError(t, err)
	assert.Equal(t, "id", withRef.Relation.Ref)

	withoutRef, err := ParseFieldTag("User", "F", "rel:belongs_to,fk:author_id")
	require.NoError(t, err)
	assert.Equal(t, "", withoutRef.Relation.Ref)
}
