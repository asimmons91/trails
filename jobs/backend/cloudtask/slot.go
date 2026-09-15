package cloudtask

import "github.com/asimmons91/trails/pack"

type slotRow struct {
	pack.Model[int64] `db:"table:cloudtask_concurrency_slots"`

	ConcurrencyKey string `db:"concurrency_key,not_null"` // unique index
	CreatedAt      int64  `db:"created_at,not_null"`
}

var slotCol = struct {
	ID             pack.Col[slotRow, int64]
	ConcurrencyKey pack.Col[slotRow, string]
	CreatedAt      pack.Col[slotRow, int64]
}{
	ID:             pack.Field(func(r *slotRow) *int64 { return &r.ID }),
	ConcurrencyKey: pack.Field(func(r *slotRow) *string { return &r.ConcurrencyKey }),
	CreatedAt:      pack.Field(func(r *slotRow) *int64 { return &r.CreatedAt }),
}
