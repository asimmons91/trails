package migrate

import (
	"context"
	"sort"
)

// Migration is one versioned schema change, written in Go against the DDL
// primitives exposed by *Migrator.
type Migration struct {
	ID       string
	Migrate  func(ctx context.Context, m *Migrator) error
	Rollback func(ctx context.Context, m *Migrator) error
}

var registered []Migration

// Register adds m to the set of known migrations. Generated migration files
// call this from an init() function, so that blank-importing a
// db/migrations package is enough to make every migration in it known to
// Up and Down.
func Register(m Migration) {
	registered = append(registered, m)
}

// Registered returns every registered migration, sorted by ID.
func Registered() []Migration {
	out := make([]Migration, len(registered))
	copy(out, registered)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
