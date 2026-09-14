package pgdialect

import (
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
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

type fakeSQLStateErr struct{ code string }

func (e *fakeSQLStateErr) Error() string    { return "driver error " + e.code }
func (e *fakeSQLStateErr) SQLState() string { return e.code }

func TestPostgres_Classify_RecognisesSQLStateError(t *testing.T) {
	code, ok := New().Classify(&fakeSQLStateErr{code: "23505"})
	assert.True(t, ok)
	assert.Equal(t, dialect.CodeUnique, code)
}

func TestPostgres_Classify_UnmappedSQLState_ReturnsFalse(t *testing.T) {
	_, ok := New().Classify(&fakeSQLStateErr{code: "42601"}) // syntax error, not a constraint code
	assert.False(t, ok)
}

func TestPostgres_Classify_UnrecognisedError_ReturnsFalse(t *testing.T) {
	_, ok := New().Classify(errors.New("boom"))
	assert.False(t, ok)
}
