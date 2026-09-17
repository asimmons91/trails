package database

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

// Migration creates the cache_entries table this backend needs. It is not
// self-registering — apps that choose the database cache backend register
// it themselves, e.g. in db/migrations:
//
//	func init() {
//		migrate.Register(database.Migration)
//	}
var Migration = migrate.Migration{
	ID: "20260917160000_create_cache_entries",

	Migrate: func(ctx context.Context, m *migrate.Migrator) error {
		if err := m.CreateTable(ctx, &entryRow{}); err != nil {
			return err
		}

		return m.CreateIndex(ctx, &entryRow{}, "idx_cache_entries_expiry",
			[]string{"ExpiresAt"})
	},

	Rollback: func(ctx context.Context, m *migrate.Migrator) error {
		return m.DropTable(ctx, &entryRow{})
	},
}
