package scan

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test fixtures ---

type nullableRow struct {
	ID    int64
	Email *string
}

func (nullableRow) TableName() string { return "nullable_rows" }

type directTypesRow struct {
	Str     string
	Boolean bool
	I       int
	I8      int8
	I16     int16
	I32     int32
	I64     int64
	U       uint
	U8      uint8
	U16     uint16
	U32     uint32
	F32     float32
	F64     float64
	Bytes   []byte
	When    time.Time
}

func (directTypesRow) TableName() string { return "direct_types_rows" }

type nullTypesRow struct {
	Name sql.NullString
	Age  sql.NullInt64
}

func (nullTypesRow) TableName() string { return "null_types_rows" }

// customCode is a hand-written driver.Valuer/sql.Scanner type (R12.4).
type customCode struct{ v string }

func (c *customCode) Scan(value any) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("customCode.Scan: unsupported type %T", value)
	}
	c.v = s
	return nil
}

func (c customCode) Value() (driver.Value, error) { return c.v, nil }

type customCodeRow struct {
	Code customCode
}

func (customCodeRow) TableName() string { return "custom_code_rows" }

// jsonbRow exercises R12.5's type:jsonb destination (jsonbDest): Tags is a
// plain Go slice with no driver.Valuer/sql.Scanner of its own.
type jsonbRow struct {
	Tags []string `db:",type:jsonb"`
}

func (jsonbRow) TableName() string { return "jsonb_rows" }

type addressMixin struct {
	City string
}

type mixinRow struct {
	ID int64
	addressMixin
}

func (mixinRow) TableName() string { return "mixin_rows" }

// --- helpers ---

func tableFor[T any](t *testing.T) *schema.Table {
	t.Helper()
	tbl, err := schema.For(reflect.TypeFor[T]())
	require.NoError(t, err)
	return tbl
}

func queryWith(t *testing.T, db *testdb.FakeDB, res testdb.Result) *sql.Rows {
	t.Helper()
	db.Enqueue(res)
	rows, err := db.Open().QueryContext(context.Background(), "SELECT ...")
	require.NoError(t, err)
	t.Cleanup(func() { rows.Close() })
	return rows
}

// --- tests ---

func TestOne_NestedPointerField_NullAndValue(t *testing.T) {
	tbl := tableFor[nullableRow](t)

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: []string{"id", "email"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), nil),
			testdb.Row(int64(2), "a@b.com"),
		},
	})

	plan := NewPlan(tbl, []string{"id", "email"})

	row1, err := One[nullableRow](rows, plan)
	require.NoError(t, err)
	assert.EqualValues(t, 1, row1.ID)
	require.Nil(t, row1.Email)

	row2, err := One[nullableRow](rows, plan)
	require.NoError(t, err)
	assert.EqualValues(t, 2, row2.ID)
	require.NotNil(t, row2.Email)
	assert.Equal(t, "a@b.com", *row2.Email)
}

func TestOne_NoRows_ReturnsSQLErrNoRows(t *testing.T) {
	tbl := tableFor[nullableRow](t)
	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{Columns: []string{"id", "email"}})

	plan := NewPlan(tbl, []string{"id", "email"})
	_, err := One[nullableRow](rows, plan)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAll_DirectTypes_R12_1(t *testing.T) {
	tbl := tableFor[directTypesRow](t)
	columns := []string{
		"str", "boolean", "i", "i8", "i16", "i32", "i64",
		"u", "u8", "u16", "u32", "f32", "f64", "bytes", "when",
	}

	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows: [][]driver.Value{
			testdb.Row("hi", true, 1, int8(2), int16(3), int32(4), int64(5),
				uint(6), uint8(7), uint16(8), uint32(9), float32(1.5), 2.5,
				[]byte("bin"), when),
		},
	})

	plan := NewPlan(tbl, columns)
	got, err := All[directTypesRow](rows, plan)
	require.NoError(t, err)
	require.Len(t, got, 1)

	want := directTypesRow{
		Str: "hi", Boolean: true,
		I: 1, I8: 2, I16: 3, I32: 4, I64: 5,
		U: 6, U8: 7, U16: 8, U32: 9,
		F32: 1.5, F64: 2.5,
		Bytes: []byte("bin"), When: when,
	}
	assert.Equal(t, want, got[0])
}

func TestOne_SQLNullTypes_R12_3(t *testing.T) {
	tbl := tableFor[nullTypesRow](t)
	columns := []string{"name", "age"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows: [][]driver.Value{
			testdb.Row(nil, nil),
			testdb.Row("bob", int64(30)),
		},
	})

	plan := NewPlan(tbl, columns)

	row1, err := One[nullTypesRow](rows, plan)
	require.NoError(t, err)
	assert.False(t, row1.Name.Valid)
	assert.False(t, row1.Age.Valid)

	row2, err := One[nullTypesRow](rows, plan)
	require.NoError(t, err)
	require.True(t, row2.Name.Valid)
	assert.Equal(t, "bob", row2.Name.String)
	require.True(t, row2.Age.Valid)
	assert.EqualValues(t, 30, row2.Age.Int64)
}

func TestOne_DriverValuerScannerPassthrough_R12_4(t *testing.T) {
	tbl := tableFor[customCodeRow](t)
	columns := []string{"code"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row("XYZ")},
	})

	plan := NewPlan(tbl, columns)
	got, err := One[customCodeRow](rows, plan)
	require.NoError(t, err)
	assert.Equal(t, "XYZ", got.Code.v)
}

func TestOne_JSONBColumn_UnmarshalsFromBytes_R12_5(t *testing.T) {
	tbl := tableFor[jsonbRow](t)
	columns := []string{"tags"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row([]byte(`["a","b"]`))},
	})

	plan := NewPlan(tbl, columns)
	got, err := One[jsonbRow](rows, plan)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got.Tags)
}

func TestOne_JSONBColumn_UnmarshalsFromString_R12_5(t *testing.T) {
	tbl := tableFor[jsonbRow](t)
	columns := []string{"tags"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row(`["x"]`)},
	})

	plan := NewPlan(tbl, columns)
	got, err := One[jsonbRow](rows, plan)
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, got.Tags)
}

func TestOne_JSONBColumn_NullBecomesZeroValue_R12_5(t *testing.T) {
	tbl := tableFor[jsonbRow](t)
	columns := []string{"tags"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row(nil)},
	})

	plan := NewPlan(tbl, columns)
	got, err := One[jsonbRow](rows, plan)
	require.NoError(t, err)
	assert.Nil(t, got.Tags)
}

func TestAll_MultiLevelEmbeddedField(t *testing.T) {
	tbl := tableFor[mixinRow](t)
	columns := []string{"id", "city"}

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row(int64(1), "Springfield")},
	})

	plan := NewPlan(tbl, columns)
	got, err := All[mixinRow](rows, plan)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, 1, got[0].ID)
	assert.Equal(t, "Springfield", got[0].City)
}

func TestAll_UnmappedExtraColumn_Discarded(t *testing.T) {
	tbl := tableFor[nullableRow](t)
	columns := []string{"id", "email", "cnt"} // "cnt" has no matching field

	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{
		Columns: columns,
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(42))},
	})

	plan := NewPlan(tbl, columns)
	got, err := All[nullableRow](rows, plan)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.EqualValues(t, 1, got[0].ID)
	require.NotNil(t, got[0].Email)
	assert.Equal(t, "a@b.com", *got[0].Email)
}

func TestAll_NoRows_ReturnsEmptyNoError(t *testing.T) {
	tbl := tableFor[nullableRow](t)
	db := testdb.New()
	rows := queryWith(t, db, testdb.Result{Columns: []string{"id", "email"}})

	plan := NewPlan(tbl, []string{"id", "email"})
	got, err := All[nullableRow](rows, plan)
	require.NoError(t, err)
	assert.Empty(t, got)
}
