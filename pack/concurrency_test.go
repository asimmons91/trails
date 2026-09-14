package pack

import (
	"context"
	"database/sql/driver"
	"sync"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestDB_ConcurrentFind_IsRaceFree(t *testing.T) {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())

	const goroutines = 32
	for range goroutines {
		fake.Enqueue(testdb.Result{
			Columns: []string{"id", "email", "age"},
			Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(30))},
		})
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Find(context.Background())
			require.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestDB_ConcurrentFindAndCreate_IsRaceFree(t *testing.T) {
	findFake := testdb.New()
	findDB := Open(findFake.Open(), pgdialect.New())

	createFake := testdb.New()
	createDB := Open(createFake.Open(), pgdialect.New())

	const goroutines = 16
	for range goroutines {
		findFake.Enqueue(testdb.Result{
			Columns: []string{"id", "email", "age"},
			Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(30))},
		})
		createFake.Enqueue(testdb.Result{
			Columns: []string{"id"},
			Rows:    [][]driver.Value{testdb.Row(int64(1))},
		})
	}

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, err := Of[testUser](findDB).Find(context.Background())
			require.NoError(t, err)
		}()
		go func() {
			defer wg.Done()
			row := &testWidget{Count: 1}
			require.NoError(t, Create(context.Background(), createDB, row))
		}()
	}
	wg.Wait()
}
