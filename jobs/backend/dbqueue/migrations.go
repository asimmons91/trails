package dbqueue

import (
	"context"

	"github.com/asimmons91/trails/pack/migrate"
)

var Migration = migrate.Migration{
	ID: "20260915000000_create_dbqueue_jobs",

	Migrate: func(ctx context.Context, m *migrate.Migrator) error {
		if err := m.CreateTable(ctx, &jobRow{}); err != nil {
			return err
		}

		if err := m.CreateIndex(ctx, &jobRow{}, "idx_dbqueue_jobs_claim",
			[]string{"State", "Queue", "ScheduledAt"}); err != nil {
			return err
		}

		return m.CreateIndex(ctx, &jobRow{}, "idx_dbqueue_jobs_cleanup",
			[]string{"State", "FinishedAt"})
	},

	Rollback: func(ctx context.Context, m *migrate.Migrator) error {
		return m.DropTable(ctx, &jobRow{})
	},
}
