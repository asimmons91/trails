package mysqldialect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMySQL_QuoteIdent_PlainIdentifier(t *testing.T) {
	assert.Equal(t, "`users`", New().QuoteIdent("users"))
}

func TestMySQL_QuoteIdent_DoublesEmbeddedBacktick(t *testing.T) {
	assert.Equal(t, "`weird``name`", New().QuoteIdent("weird`name"))
}

func TestMySQL_Placeholder_AlwaysPlainQuestionMark(t *testing.T) {
	d := New()
	assert.Equal(t, "?", d.Placeholder(1))
	assert.Equal(t, "?", d.Placeholder(2))
	assert.Equal(t, "?", d.Placeholder(10))
}

func TestMySQL_Name(t *testing.T) {
	assert.Equal(t, "mysql", New().Name())
}

func TestMySQL_DoesNotSupportILike(t *testing.T) {
	assert.False(t, New().SupportsILike())
}

func TestMySQL_SupportsRowLocking(t *testing.T) {
	assert.True(t, New().SupportsRowLocking())
}

func TestMySQL_DoesNotSupportReturning(t *testing.T) {
	assert.False(t, New().SupportsReturning())
}

func TestMySQL_DoesNotSupportOnConflict(t *testing.T) {
	assert.False(t, New().SupportsOnConflict())
}
