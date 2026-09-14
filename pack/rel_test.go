package pack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelation_NilReturningSelector_Panics(t *testing.T) {
	assert.Panics(t, func() {
		Relation(func(a *testAccount) *[]*testPost { return nil })
	})
}

func TestRelationOne_NilReturningSelector_Panics(t *testing.T) {
	assert.Panics(t, func() {
		RelationOne(func(a *testAccount) **testProfile { return nil })
	})
}

func TestRelation_SelectorOutsideStruct_Panics(t *testing.T) {
	var external []*testPost
	assert.Panics(t, func() {
		Relation(func(a *testAccount) *[]*testPost {
			_ = a
			return &external
		})
	})
}
