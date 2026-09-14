package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestOpen_RoundTripsCallerOwnedDB(t *testing.T) {
	fake := testdb.New()
	sqlDB := fake.Open()
	defer sqlDB.Close()

	db := Open(sqlDB, pgdialect.New())
	require.NotNil(t, db)

	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})
	_, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 1)
}
