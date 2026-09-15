package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type migAccount struct {
	pack.Model[int64] `db:"table:mig_accounts"`
	Orders            []*migOrder `db:"rel:has_many,fk:account_id"`
}

func TestCreateConstraint_BelongsTo_DefaultName(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).CreateConstraint(context.Background(), &migOrder{}, "User")
	require.NoError(t, err)
	assert.Equal(t,
		`ALTER TABLE "orders" ADD CONSTRAINT "fk_orders_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`,
		fake.Executed()[0].SQL,
	)
}

func TestCreateConstraint_WithConstraintName_Overrides(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).CreateConstraint(context.Background(), &migOrder{}, "User", WithConstraintName("fk_custom"))
	require.NoError(t, err)
	assert.Equal(t,
		`ALTER TABLE "orders" ADD CONSTRAINT "fk_custom" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`,
		fake.Executed()[0].SQL,
	)
}

func TestCreateConstraint_RelationNotFound_ReturnsError(t *testing.T) {
	db, _ := newMigTestDB()
	err := New(db).CreateConstraint(context.Background(), &migOrder{}, "DoesNotExist")
	require.Error(t, err)
	var notFound *ErrRelationNotFound
	require.ErrorAs(t, err, &notFound)
}

func TestCreateConstraint_WrongRelationKind_ReturnsError(t *testing.T) {
	db, _ := newMigTestDB()
	err := New(db).CreateConstraint(context.Background(), &migAccount{}, "Orders")
	require.Error(t, err)
	var wrongKind *ErrConstraintWrongRelationKind
	require.ErrorAs(t, err, &wrongKind)
}

func TestDropConstraint_Idempotent_DropsWhenConstraintExists(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})
	fake.Enqueue(testdb.Result{})

	err := New(db).DropConstraint(context.Background(), &migOrder{}, "fk_orders_user_id")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "orders" DROP CONSTRAINT "fk_orders_user_id"`, fake.Executed()[1].SQL)
}

func TestDropConstraint_Idempotent_NoOpsWhenConstraintAbsent(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(false)}})

	err := New(db).DropConstraint(context.Background(), &migOrder{}, "fk_orders_user_id")
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 1)
}

func TestHasConstraint_ScansExistsColumn(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})

	exists, err := New(db).HasConstraint(context.Background(), &migOrder{}, "fk_orders_user_id")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateConstraint_SQLite_Unsupported(t *testing.T) {
	db, _ := newMigTestSQLiteDB()
	err := New(db).CreateConstraint(context.Background(), &migOrder{}, "User")
	require.Error(t, err)
}
