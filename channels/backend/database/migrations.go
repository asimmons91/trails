package database

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

var Migration = migrate.Migration{
	ID: "20260915000000_create_channel_messages",

	Migrate: func(ctx context.Context, m *migrate.Migrator) error {
		if err := m.CreateTable(ctx, &messageRow{}); err != nil {
			return err
		}

		return m.CreateIndex(ctx, &messageRow{}, "idx_channel_messages_created_at",
			[]string{"CreatedAt"})
	},

	Rollback: func(ctx context.Context, m *migrate.Migrator) error {
		return m.DropTable(ctx, &messageRow{})
	},
}
