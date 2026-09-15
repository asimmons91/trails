package database

import "github.com/asimmons91/trails/pack"

type messageRow struct {
	pack.Model[int64] `db:"table:channel_messages"`

	Topic     string `db:"topic,not_null"`
	Payload   string `db:"payload,not_null"`
	CreatedAt int64  `db:"created_at,not_null"`
}

var messageCol = struct {
	ID        pack.Col[messageRow, int64]
	Topic     pack.Col[messageRow, string]
	Payload   pack.Col[messageRow, string]
	CreatedAt pack.Col[messageRow, int64]
}{
	ID:        pack.Field(func(r *messageRow) *int64 { return &r.ID }),
	Topic:     pack.Field(func(r *messageRow) *string { return &r.Topic }),
	Payload:   pack.Field(func(r *messageRow) *string { return &r.Payload }),
	CreatedAt: pack.Field(func(r *messageRow) *int64 { return &r.CreatedAt }),
}
