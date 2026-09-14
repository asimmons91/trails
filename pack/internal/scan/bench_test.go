package scan

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"testing"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/testdb"
)

type benchRow struct {
	ID    int64
	Email string
	Age   int
}

func (benchRow) TableName() string { return "bench_rows" }

func openSingleRow(b *testing.B) *sql.Rows {
	b.Helper()
	db := testdb.New()
	db.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(30))},
	})
	rows, err := db.Open().QueryContext(b.Context(), "SELECT ...")
	if err != nil {
		b.Fatal(err)
	}
	return rows
}

// BenchmarkOne_ScanPackage measures internal/scan's One against a
// hand-written database/sql scan of the same row shape. Scanning a
// single-row query must stay within 2x of hand-written database/sql).
func BenchmarkOne_ScanPackage(b *testing.B) {
	tbl, err := schema.For(reflect.TypeFor[benchRow]())
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		rows := openSingleRow(b)
		plan := NewPlan(tbl, []string{"id", "email", "age"})
		if _, err := One[benchRow](rows, plan); err != nil {
			b.Fatal(err)
		}
		rows.Close()
	}
}

// BenchmarkOne_HandWritten is the comparison baseline: an ordinary
// database/sql scan with no reflection.
func BenchmarkOne_HandWritten(b *testing.B) {
	for b.Loop() {
		rows := openSingleRow(b)
		var row benchRow
		if !rows.Next() {
			b.Fatal("expected one row")
		}
		if err := rows.Scan(&row.ID, &row.Email, &row.Age); err != nil {
			b.Fatal(err)
		}
		rows.Close()
	}
}
