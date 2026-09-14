package sqlitedialect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSQLite_QuoteIdent_PlainIdentifier(t *testing.T) {
	assert.Equal(t, `"users"`, New().QuoteIdent("users"))
}

func TestSQLite_QuoteIdent_DoublesEmbeddedQuote(t *testing.T) {
	assert.Equal(t, `"weird""name"`, New().QuoteIdent(`weird"name`))
}

func TestSQLite_Placeholder_SequentialOrdinals(t *testing.T) {
	d := New()
	assert.Equal(t, "?1", d.Placeholder(1))
	assert.Equal(t, "?2", d.Placeholder(2))
	assert.Equal(t, "?10", d.Placeholder(10))
}

func TestSQLite_Name(t *testing.T) {
	assert.Equal(t, "sqlite", New().Name())
}

func TestSQLite_DoesNotSupportILike(t *testing.T) {
	assert.False(t, New().SupportsILike())
}

func TestSQLite_DoesNotSupportRowLocking(t *testing.T) {
	assert.False(t, New().SupportsRowLocking())
}

func TestSQLite_SupportsReturning(t *testing.T) {
	assert.True(t, New().SupportsReturning())
}

func TestSQLite_SupportsOnConflict(t *testing.T) {
	assert.True(t, New().SupportsOnConflict())
}
