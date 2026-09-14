package pgdialect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPostgres_QuoteIdent_PlainIdentifier(t *testing.T) {
	assert.Equal(t, `"users"`, New().QuoteIdent("users"))
}

func TestPostgres_QuoteIdent_DoublesEmbeddedQuote(t *testing.T) {
	assert.Equal(t, `"weird""name"`, New().QuoteIdent(`weird"name`))
}

func TestPostgres_Placeholder_SequentialOrdinals(t *testing.T) {
	d := New()
	assert.Equal(t, "$1", d.Placeholder(1))
	assert.Equal(t, "$2", d.Placeholder(2))
	assert.Equal(t, "$10", d.Placeholder(10))
}

func TestPostgres_Name(t *testing.T) {
	assert.Equal(t, "postgres", New().Name())
}

func TestPostgres_SupportsReturning(t *testing.T) {
	assert.True(t, New().SupportsReturning())
}

func TestPostgres_SupportsOnConflict(t *testing.T) {
	assert.True(t, New().SupportsOnConflict())
}
