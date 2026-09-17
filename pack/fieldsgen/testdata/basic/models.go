package models

import (
	"time"

	"github.com/asimmons91/trails/pack"
)

type addressMixin struct {
	City string `db:"city"`
}

type Author struct {
	pack.Model[int64] `db:"table:authors"`
	addressMixin
	Name    string         `db:"name,not_null"`
	Secret  string         `db:"-"`
	Posts   []*Post        `db:"rel:has_many,fk:author_id"`
	Profile *AuthorProfile `db:"rel:has_one,fk:author_id"`
}

type AuthorProfile struct {
	pack.Model[int64] `db:"table:author_profiles"`
	AuthorID          int64  `db:"author_id"`
	Bio               string `db:"bio"`
}

type Post struct {
	pack.Model[int64] `db:"table:posts"`
	AuthorID          int64     `db:"author_id"`
	Title             string    `db:"title,not_null"`
	CreatedAt         time.Time `db:",default:now()"`
	Author            *Author   `db:"rel:belongs_to,fk:author_id"`
}
