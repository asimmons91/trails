package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionSetGetDelete(t *testing.T) {
	s := New()
	require.Nil(t, s.Get("key"))

	s.Set("key", "value")
	require.Equal(t, "value", s.Get("key"))
	require.True(t, s.isDirty())

	s.Delete("key")
	require.Nil(t, s.Get("key"))
}

func TestSessionDeleteMissingKeyDoesNotMarkDirty(t *testing.T) {
	s := New()
	s.Delete("missing")
	require.False(t, s.isDirty())
}

func TestSessionClearMarksDirtyOnlyIfNonEmpty(t *testing.T) {
	s := New()
	s.Clear()
	require.False(t, s.isDirty())

	s.Set("key", "value")
	s2 := &Session{data: s.snapshot()}
	s2.Clear()
	require.True(t, s2.isDirty())
	require.True(t, s2.isEmpty())
}

func TestSessionCSRFTokenIsStableAcrossReads(t *testing.T) {
	s := New()
	first := s.CSRFToken()
	require.True(t, s.isDirty())

	// isDirty should reflect the first generation; verify a second read
	// returns the identical token without changing it.
	second := s.CSRFToken()
	require.Equal(t, first, second)
}

func TestSessionCSRFTokenSelfHealsOnCorruptStoredValue(t *testing.T) {
	s := New()
	s.data[csrfTokenKey] = "not-valid-base64!!"

	token := s.CSRFToken()
	require.Len(t, token, csrfTokenLen)
}

func TestSessionCSRFTokenSelfHealsOnWrongLength(t *testing.T) {
	s := New()
	s.data[csrfTokenKey] = "aGVsbG8=" // valid base64, wrong decoded length

	token := s.CSRFToken()
	require.Len(t, token, csrfTokenLen)
}

func TestSessionSnapshotIsIndependentCopy(t *testing.T) {
	s := New()
	s.Set("key", "value")

	snap := s.snapshot()
	snap["key"] = "mutated"

	require.Equal(t, "value", s.Get("key"))
}
