package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// BCryptCost is the work factor SecurePassword hashes new passwords with.
// It defaults to bcrypt.DefaultCost; lower it (e.g. to bcrypt.MinCost) in
// tests that create many users and don't need production-strength hashing
// speed.
var BCryptCost = bcrypt.DefaultCost

// ErrPasswordRequired is returned from SecurePassword.BeforeInsert when a
// new record has neither Password nor a pre-populated PasswordDigest set.
var ErrPasswordRequired = errors.New("auth: password is required")

// ErrPasswordConfirmationMismatch is returned when both Password and
// PasswordConfirmation are set but don't match.
var ErrPasswordConfirmationMismatch = errors.New("auth: password confirmation does not match password")

// SecurePassword is a has_secure_password-style mixin: embed it in a pack
// model to get bcrypt password hashing on create/update and an Authenticate
// method, for free.
//
//	type User struct {
//	    pack.Model[int64] `db:"table:users"`
//	    Email string `db:"email,unique,not_null"`
//	    auth.SecurePassword
//	}
//
//	u := &User{Email: "a@b.com"}
//	u.Password = "hunter2"
//	u.PasswordConfirmation = "hunter2"
//	pack.Create(ctx, db, u) // hashes Password into PasswordDigest automatically
//	u.Authenticate("hunter2") // true
//
// Password and PasswordConfirmation are virtual (`db:"-"`): they're never
// persisted and are cleared after a successful hash. Leaving Password blank
// on an update leaves the stored PasswordDigest untouched.
//
// Go has no callback chaining, so if the embedding model needs its own
// BeforeInsert/BeforeUpdate too, defining one on the model shadows this
// one entirely — delegate to it explicitly:
//
//	func (u *User) BeforeInsert(ctx context.Context) error {
//	    if err := u.SecurePassword.BeforeInsert(ctx); err != nil {
//	        return err
//	    }
//	    // ... other logic ...
//	    return nil
//	}
type SecurePassword struct {
	PasswordDigest string `db:"password_digest,not_null"`

	Password             string `db:"-"`
	PasswordConfirmation string `db:"-"`
}

// BeforeInsert hashes Password into PasswordDigest. It errors if neither
// Password nor a pre-populated PasswordDigest is set, or if
// PasswordConfirmation is set but doesn't match Password.
func (p *SecurePassword) BeforeInsert(ctx context.Context) error {
	return p.hashPending(true)
}

// BeforeUpdate hashes Password into PasswordDigest if Password is set; if
// it's left blank, the existing PasswordDigest is left untouched.
func (p *SecurePassword) BeforeUpdate(ctx context.Context) error {
	return p.hashPending(false)
}

func (p *SecurePassword) hashPending(requireOnCreate bool) error {
	if p.Password == "" {
		if requireOnCreate && p.PasswordDigest == "" {
			return ErrPasswordRequired
		}
		return nil
	}

	if p.PasswordConfirmation != "" && p.PasswordConfirmation != p.Password {
		return ErrPasswordConfirmationMismatch
	}

	digest, err := bcrypt.GenerateFromPassword([]byte(p.Password), BCryptCost)
	if err != nil {
		return fmt.Errorf("auth: hashing password: %w", err)
	}

	p.PasswordDigest = string(digest)
	p.Password = ""
	p.PasswordConfirmation = ""

	return nil
}

// dummyDigest is hashed once, on first use, at whatever BCryptCost is set
// to at that point. Authenticate compares against it when PasswordDigest is
// empty so that path costs the same as a real comparison.
var dummyDigest = sync.OnceValue(func() string {
	digest, err := bcrypt.GenerateFromPassword([]byte("trails-dummy-password-for-timing-safety"), BCryptCost)
	if err != nil {
		return ""
	}
	return string(digest)
})

// Authenticate reports whether password matches the stored PasswordDigest.
//
// It always runs a bcrypt comparison, even when PasswordDigest is empty
// (e.g. the caller looked up a user that doesn't exist and got a
// zero-value struct). Without this, a caller doing the common
// `user, _ := findByEmail(email); user.Authenticate(password)` pattern
// would return near-instantly for a nonexistent user but take the full
// bcrypt cost for a real one with a wrong password — a timing oracle for
// user enumeration.
func (p *SecurePassword) Authenticate(password string) bool {
	digest := p.PasswordDigest
	if digest == "" {
		digest = dummyDigest()
	}

	match := bcrypt.CompareHashAndPassword([]byte(digest), []byte(password)) == nil
	return match && p.PasswordDigest != ""
}
