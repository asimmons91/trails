package sqlbuild

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInsert_BasicPlaceholdersInAssignmentOrder(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email", "age") VALUES ($1, $2)`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email", "age") VALUES (?1, ?2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(
					Set(Col("email"), "a@b.com"),
					Set(Col("age"), 30),
				).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 30}, args)
		})
	}
}

func TestInsert_Returning(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1) RETURNING "id", "created_at"`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1) RETURNING "id", "created_at"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com")).
				Returning(Col("id"), Col("created_at")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com"}, args)
		})
	}
}

func TestInsert_ColumnOrder_MatchesAssignmentOrder(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("z_last", "a_first") VALUES ($1, $2)`},
		{"SQLite", sqlite, `INSERT INTO "users" ("z_last", "a_first") VALUES (?1, ?2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Insert(Table{Name: "users"}).
				Values(
					Set(Col("z_last"), 1),
					Set(Col("a_first"), 2),
				).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestInsert_BuiltOnTwice_DoesNotLeakBetweenDerivedBuilders(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, ""},
		{"SQLite", sqlite, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com"))

			withReturning, _, err := base.Returning(Col("id")).Render(tc.d)
			require.NoError(t, err)

			baseAgain, _, err := base.Render(tc.d)
			require.NoError(t, err)

			switch tc.name {
			case "Postgres":
				require.Equal(t, `INSERT INTO "users" ("email") VALUES ($1) RETURNING "id"`, withReturning)
				require.Equal(t, `INSERT INTO "users" ("email") VALUES ($1)`, baseAgain)
			case "SQLite":
				require.Equal(t, `INSERT INTO "users" ("email") VALUES (?1) RETURNING "id"`, withReturning)
				require.Equal(t, `INSERT INTO "users" ("email") VALUES (?1)`, baseAgain)
			}
		})
	}
}

func TestInsert_MultiRow_RendersOneValuesClausePerRow(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email", "age") VALUES ($1, $2), ($3, $4), ($5, $6)`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email", "age") VALUES (?1, ?2), (?3, ?4), (?5, ?6)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com"), Set(Col("age"), 30)).
				Values(Set(Col("email"), "c@d.com"), Set(Col("age"), 40)).
				Values(Set(Col("email"), "e@f.com"), Set(Col("age"), 50)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 30, "c@d.com", 40, "e@f.com", 50}, args)
		})
	}
}

func TestInsert_MultiRow_WithReturning(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1), ($2) RETURNING "id"`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1), (?2) RETURNING "id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com")).
				Values(Set(Col("email"), "c@d.com")).
				Returning(Col("id")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", "c@d.com"}, args)
		})
	}
}

func TestInsert_AssignmentTarget_IsNeverQualified_EvenIfColumnCarriesAnAlias(t *testing.T) {
	// A qualified Column (as Col[T,F].Set produces) must still render an
	// unqualified INSERT column list — Postgres never accepts a qualified
	// column here, in any context.
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1)`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Insert(Table{Name: "users"}).
				Values(Set(QualifiedCol("u", "email"), "a@b.com")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestInsert_OnConflictDoNothing(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1) ON CONFLICT ("email") DO NOTHING`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1) ON CONFLICT ("email") DO NOTHING`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com")).
				OnConflictDoNothing(Col("email")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com"}, args)
		})
	}
}

func TestInsert_OnConflictDoUpdate(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email", "name") VALUES ($1, $2) ON CONFLICT ("email") DO UPDATE SET "name" = EXCLUDED.name`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email", "name") VALUES (?1, ?2) ON CONFLICT ("email") DO UPDATE SET "name" = EXCLUDED.name`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com"), Set(Col("name"), "Bob")).
				OnConflictDoUpdate(
					[]Column{Col("email")},
					SetExpr(Col("name"), "EXCLUDED.name"),
				).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", "Bob"}, args)
		})
	}
}

func TestInsert_OnConflictDoUpdate_TypedAssignment(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1) ON CONFLICT ("email") DO UPDATE SET "login_count" = $2`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1) ON CONFLICT ("email") DO UPDATE SET "login_count" = ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com")).
				OnConflictDoUpdate(
					[]Column{Col("email")},
					Set(Col("login_count"), 1),
				).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 1}, args)
		})
	}
}

func TestInsert_OnConflictAndReturning_ClauseOrder(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `INSERT INTO "users" ("email") VALUES ($1) ON CONFLICT ("email") DO NOTHING RETURNING "id"`},
		{"SQLite", sqlite, `INSERT INTO "users" ("email") VALUES (?1) ON CONFLICT ("email") DO NOTHING RETURNING "id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Insert(Table{Name: "users"}).
				Values(Set(Col("email"), "a@b.com")).
				OnConflictDoNothing(Col("email")).
				Returning(Col("id")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}
