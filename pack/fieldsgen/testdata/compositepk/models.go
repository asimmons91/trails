package models

import "github.com/asimmons91/trails/pack"

// MemberKey is a composite primary key: two scalar fields packed into a
// struct, used as the ID type argument to pack.Model.
type MemberKey struct {
	OrgID  int64 `db:"org_id"`
	UserID int64 `db:"user_id"`
}

// Membership has a composite-key ID, plus an ordinary scalar column and a
// relation, so the golden output for this fixture must show only the ID
// Col entry excluded — Role and the Notes relation still generate.
type Membership struct {
	pack.Model[MemberKey] `db:"table:memberships"`
	Role                  string  `db:"role,not_null"`
	Notes                 []*Note `db:"rel:has_many,fk:user_id"`
}

type Note struct {
	pack.Model[int64] `db:"table:notes"`
	Body              string `db:"body"`
}
