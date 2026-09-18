package cloudtask

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

// Migration creates the cloudtask_jobs, cloudtask_concurrency_slots, and
// cloudtask_schedule tables and their indexes. Apply it (e.g. via
// pack/migrate) against the app's database before constructing a Backend
// against it.
var Migration = migrate.Migration{
	ID: "20260915000000_create_cloudtask_tables",

	Migrate: func(ctx context.Context, m *migrate.Migrator) error {
		if err := m.CreateTable(ctx, &jobRow{}); err != nil {
			return err
		}
		if err := m.CreateIndex(ctx, &jobRow{}, "idx_cloudtask_jobs_concurrency",
			[]string{"ConcurrencyKey", "State", "LockedAt"}); err != nil {
			return err
		}
		if err := m.CreateIndex(ctx, &jobRow{}, "idx_cloudtask_jobs_cleanup",
			[]string{"State", "FinishedAt"}); err != nil {
			return err
		}

		if err := m.CreateTable(ctx, &slotRow{}); err != nil {
			return err
		}
		if err := m.CreateIndex(ctx, &slotRow{}, "idx_cloudtask_slots_key",
			[]string{"ConcurrencyKey"}, migrate.WithUniqueIndex()); err != nil {
			return err
		}

		if err := m.CreateTable(ctx, &scheduleRow{}); err != nil {
			return err
		}
		return m.CreateIndex(ctx, &scheduleRow{}, "idx_cloudtask_schedule_name",
			[]string{"Name"}, migrate.WithUniqueIndex())
	},

	Rollback: func(ctx context.Context, m *migrate.Migrator) error {
		if err := m.DropTable(ctx, &scheduleRow{}); err != nil {
			return err
		}
		if err := m.DropTable(ctx, &slotRow{}); err != nil {
			return err
		}
		return m.DropTable(ctx, &jobRow{})
	},
}
