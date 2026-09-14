package pack

import (
	"context"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
)

func Example() {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())

	fake.Enqueue(testdb.Result{ // the account itself
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "ada@example.com", time.Time{}, ""),
		},
	})
	fake.Enqueue(testdb.Result{ // Posts, batch-loaded by Preload (R9.5)
		Columns: []string{"id", "user_id", "title"},
		Rows: [][]driver.Value{
			testdb.Row(int64(10), int64(1), "Hello, Argo"),
		},
	})

	accounts, err := Of[testAccount](db).
		Where(accountCol.Email.Eq("ada@example.com")).
		Preload(accountRel.Posts).
		Find(context.Background())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(accounts[0].Email, len(accounts[0].Posts), accounts[0].Posts[0].Title)

	fake.Enqueue(testdb.Result{ // Create's RETURNING id + the default: created_at column
		Columns: []string{"id", "created_at"},
		Rows:    [][]driver.Value{testdb.Row(int64(2), time.Time{})},
	})
	row := &testAccount{Email: "grace@example.com"}
	if err := Create(context.Background(), db, row); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(row.ID)

	// Output:
	// ada@example.com 1 Hello, Argo
	// 2
}

func ExampleDB_Tx() {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	err := db.Tx(context.Background(), func(tx *DB) error {
		return Create(context.Background(), tx, &testWidget{Count: 1})
	})
	fmt.Println(err)
	// Output:
	// <nil>
}

func ExampleCreateAll() {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1)), testdb.Row(int64(2))},
	})

	rows := []*testWidget{{Count: 10}, {Count: 20}}
	if err := CreateAll(context.Background(), db, rows); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(rows[0].ID, rows[1].ID)
	// Output:
	// 1 2
}

func ExampleOnConflict() {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	row := &testWidget{Count: 5}
	err := Create(context.Background(), db, row, OnConflict[testWidget](widgetCol.ID).DoNothing())
	fmt.Println(err)
	// Output:
	// <nil>
}

func ExampleQuery_Rows() {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
		},
	})

	for row, err := range Of[testHookedItem](db).Rows(context.Background()) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(row.ID, row.Name)
	}
	// Output:
	// 1 a
	// 2 b
}
