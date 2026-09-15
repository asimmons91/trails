package sqlbuild

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateForeignKeyConstraint_PerDialect(t *testing.T) {
	build := func() *CreateConstraintBuilder {
		return CreateForeignKeyConstraint(Table{Name: "orders"}, "fk_orders_user_id", []string{"user_id"}, "users", []string{"id"})
	}

	for _, tc := range []dialectCase{
		{"Postgres", pg, `ALTER TABLE "orders" ADD CONSTRAINT "fk_orders_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`},
		{"MySQL", mysql, "ALTER TABLE `orders` ADD CONSTRAINT `fk_orders_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := build().Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

// SQLite's ALTER TABLE grammar has no ADD CONSTRAINT / DROP CONSTRAINT form
// at all - a foreign key can only be declared inline in CREATE TABLE.
func TestCreateForeignKeyConstraint_SQLite_Unsupported(t *testing.T) {
	_, _, err := CreateForeignKeyConstraint(Table{Name: "orders"}, "fk_orders_user_id", []string{"user_id"}, "users", []string{"id"}).Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrConstraintsUnsupportedByDialect{}, err)
}

func TestDropConstraint_SQLite_Unsupported(t *testing.T) {
	_, _, err := DropConstraint(Table{Name: "orders"}, "fk_orders_user_id").Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrConstraintsUnsupportedByDialect{}, err)
}

func TestDropConstraint_MySQL_UsesDropForeignKey(t *testing.T) {
	sql, _, err := DropConstraint(Table{Name: "orders"}, "fk_orders_user_id").Render(mysql)
	require.NoError(t, err)
	require.Equal(t, "ALTER TABLE `orders` DROP FOREIGN KEY `fk_orders_user_id`", sql)
}

func TestDropConstraint_Postgres_UsesDropConstraint(t *testing.T) {
	sql, _, err := DropConstraint(Table{Name: "orders"}, "fk_orders_user_id").Render(pg)
	require.NoError(t, err)
	require.Equal(t, `ALTER TABLE "orders" DROP CONSTRAINT "fk_orders_user_id"`, sql)
}
