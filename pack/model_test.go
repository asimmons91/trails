package pack

import (
	"reflect"
	"testing"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_EmbeddedGivesEntitySatisfaction(t *testing.T) {
	var e Entity[int64] = &testAccount{}
	_ = e

	acc := testAccount{}
	acc.SetPK(42)
	assert.EqualValues(t, 42, acc.PK())
}

func TestModel_PromotedFieldLiteral_NoNestedStruct(t *testing.T) {
	acc := testAccount{Email: "a@b.com"}
	acc.ID = 7
	assert.EqualValues(t, 7, acc.ID)
	assert.EqualValues(t, 7, acc.PK())
	assert.Equal(t, "a@b.com", acc.Email)
}

func TestSchema_CompositeKey_ExpandsInDeclarationOrder(t *testing.T) {
	tbl, err := schema.For(reflect.TypeFor[testMembership]())
	require.NoError(t, err)
	require.True(t, tbl.PKIsComposite)
	require.Len(t, tbl.PK, 2)
	assert.Equal(t, "org_id", tbl.PK[0].Column)
	assert.Equal(t, "user_id", tbl.PK[1].Column)
}
