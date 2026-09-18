// Package session provides cookie-store sessions for trails:
// the entire session is serialized and encrypted directly into a single
// cookie, with no server-side storage. See Middleware.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

// csrfTokenKey is the reserved session key the csrf package's token lives
// under. It is unexported so callers can't collide with it via Set/Delete.
const csrfTokenKey = "_csrf_token"

const csrfTokenLen = 32

// Session is a request's session data. It is not safe to retain across
// requests; fetch it fresh via FromContext each time.
//
// Values read back after a cookie round-trip come back as
// encoding/json's generic decode types (string, float64, bool, []any,
// map[string]any, nil) — the same caveat any interface{} JSON decode has.
// A value Set and then Get within the same request keeps its original type.
type Session struct {
	mu    sync.Mutex
	data  map[string]any
	dirty bool
}

// New returns an empty session.
func New() *Session {
	return &Session{data: map[string]any{}}
}

// Get returns the value stored under key, or nil if absent.
func (s *Session) Get(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.data[key]
}

// Set stores val under key, marking the session dirty.
func (s *Session) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = val
	s.dirty = true
}

// Delete removes key, marking the session dirty only if key was present.
func (s *Session) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data[key]; ok {
		delete(s.data, key)
		s.dirty = true
	}
}

// Clear removes all values, marking the session dirty only if it was
// non-empty.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.data) > 0 {
		s.data = map[string]any{}
		s.dirty = true
	}
}

// isDirty reports whether the session has changed since it was loaded.
func (s *Session) isDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.dirty
}

// isEmpty reports whether the session currently holds no data.
func (s *Session) isEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.data) == 0
}

// snapshot returns a copy of the session's data suitable for JSON encoding.
func (s *Session) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}

	return out
}

// CSRFToken returns this session's per-session random CSRF secret,
// generating and persisting it on first access. The csrf package calls this
// via FromContext(c).CSRFToken() — it is not meant to be read directly by
// application code.
func (s *Session) CSRFToken() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if raw, ok := s.data[csrfTokenKey].(string); ok {
		if token, err := base64.StdEncoding.DecodeString(raw); err == nil && len(token) == csrfTokenLen {
			return token
		}
		// Corrupt/wrong-length stored value — self-heal below rather than
		// ever failing a request over a bad CSRF token.
	}

	token := make([]byte, csrfTokenLen)
	if _, err := rand.Read(token); err != nil {
		// crypto/rand.Read only fails if the OS CSPRNG is broken, which is
		// unrecoverable for anything security-sensitive in this process.
		panic("session: reading random bytes for CSRF token: " + err.Error())
	}

	s.data[csrfTokenKey] = base64.StdEncoding.EncodeToString(token)
	s.dirty = true

	return token
}
