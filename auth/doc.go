// Package auth provides small, composable authentication primitives for
// trails models: bcrypt password hashing (SecurePassword), random opaque
// tokens for columns like API keys (NewToken/EnsureToken), and signed,
// expiring, purpose-scoped tokens for flows like password resets
// (GenerateTokenFor/VerifyTokenFor).
//
// None of these require any changes to the pack ORM — they work entirely
// through the existing pack.BeforeInserter/BeforeUpdater hook interfaces
// (see pack/hooks.go) and the `db:"-"` convention for virtual, non-persisted
// struct fields.
package auth
