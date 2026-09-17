package database

import "github.com/asimmons91/trails/pack"

type entryRow struct {
	pack.Model[int64] `db:"table:cache_entries"`

	Key       string `db:"key,not_null,unique"`
	Value     []byte `db:"value,not_null"`
	ExpiresAt int64  `db:"expires_at,not_null"` // unix nanos; 0 means never expires
	CreatedAt int64  `db:"created_at,not_null"`
}

var entryCol = struct {
	ID        pack.Col[entryRow, int64]
	Key       pack.Col[entryRow, string]
	Value     pack.Col[entryRow, []byte]
	ExpiresAt pack.Col[entryRow, int64]
	CreatedAt pack.Col[entryRow, int64]
}{
	ID:        pack.Field(func(r *entryRow) *int64 { return &r.ID }),
	Key:       pack.Field(func(r *entryRow) *string { return &r.Key }),
	Value:     pack.Field(func(r *entryRow) *[]byte { return &r.Value }),
	ExpiresAt: pack.Field(func(r *entryRow) *int64 { return &r.ExpiresAt }),
	CreatedAt: pack.Field(func(r *entryRow) *int64 { return &r.CreatedAt }),
}
