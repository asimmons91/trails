package database

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

// Migration creates the channel_messages table this backend needs. It is
// not self-registering — apps that choose the database channels backend
// register it themselves, e.g. in db/migrations:
//
//	func init() {
//		migrate.Register(database.Migration)
//	}
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
