package models

import "github.com/asimmons91/trails/pack"

type Example struct {
	pack.Model[int64] `db:"table:examples"`

	Title string `db:"title,not_null"`
}
