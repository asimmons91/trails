package sqlitedialect

import (
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
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

type fakeCodeErr struct{ code int }

func (e *fakeCodeErr) Error() string { return "driver error" }
func (e *fakeCodeErr) Code() int     { return e.code }

func TestSQLite_Classify_MapsExtendedResultCodes(t *testing.T) {
	cases := []struct {
		name string
		code int
		want string
	}{
		{"unique", 2067, dialect.CodeUnique},
		{"primary key", 1555, dialect.CodeUnique},
		{"foreign key", 787, dialect.CodeForeignKey},
		{"not null", 1299, dialect.CodeNotNull},
		{"check", 275, dialect.CodeCheck},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := New().Classify(&fakeCodeErr{code: c.code})
			assert.True(t, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestSQLite_Classify_UnmappedCode_ReturnsFalse(t *testing.T) {
	_, ok := New().Classify(&fakeCodeErr{code: 1}) // SQLITE_ERROR, not a constraint code
	assert.False(t, ok)
}

func TestSQLite_Classify_UnrecognisedError_ReturnsFalse(t *testing.T) {
	_, ok := New().Classify(errors.New("boom"))
	assert.False(t, ok)
}
