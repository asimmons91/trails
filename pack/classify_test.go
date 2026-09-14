package pack

import (
	"context"
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSQLStateErr struct{ code string }

func (e *fakeSQLStateErr) Error() string    { return "driver error " + e.code }
func (e *fakeSQLStateErr) SQLState() string { return e.code }

func TestCreate_UniqueViolation_ClassifiedAsTypedError(t *testing.T) {
	db, fake := newTestDB()
	driverErr := &fakeSQLStateErr{code: "23505"}
	fake.Enqueue(testdb.Result{Err: driverErr})

	row := &testWidget{Count: 1}
	err := Create(context.Background(), db, row)
	require.Error(t, err)

	v, ok := errors.AsType[*ErrUniqueViolation](err)
	require.True(t, ok)
	assert.Equal(t, "testWidget", v.Model)
	assert.Equal(t, "Create", v.Operation)
	assert.ErrorIs(t, err, driverErr, "Unwrap must still reach the original driver error")
}

func TestClassifyError_AllFourSQLStateCodes(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{"23505", "UniqueViolation"},
		{"23503", "ForeignKeyViolation"},
		{"23502", "NotNullViolation"},
		{"23514", "CheckViolation"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			db, fake := newTestDB()
			fake.Enqueue(testdb.Result{Err: &fakeSQLStateErr{code: c.code}})

			_, err := Of[testWidget](db).Where(widgetCol.ID.Eq(int64(1))).Delete(context.Background())
			require.Error(t, err)

			switch c.want {
			case "UniqueViolation":
				_, ok := errors.AsType[*ErrUniqueViolation](err)
				assert.True(t, ok)
			case "ForeignKeyViolation":
				_, ok := errors.AsType[*ErrForeignKeyViolation](err)
				assert.True(t, ok)
			case "NotNullViolation":
				_, ok := errors.AsType[*ErrNotNullViolation](err)
				assert.True(t, ok)
			case "CheckViolation":
				_, ok := errors.AsType[*ErrCheckViolation](err)
				assert.True(t, ok)
			}
		})
	}
}

func TestClassifyError_UnmappedSQLState_DegradesToOriginalError(t *testing.T) {
	db, fake := newTestDB()
	driverErr := &fakeSQLStateErr{code: "42601"} // syntax error, not a §13 constraint code
	fake.Enqueue(testdb.Result{Err: driverErr})

	_, err := Of[testWidget](db).Where(widgetCol.ID.Eq(int64(1))).Delete(context.Background())
	require.Error(t, err)
	assert.Same(t, driverErr, err)

	_, ok := errors.AsType[*ErrUniqueViolation](err)
	assert.False(t, ok)
}

func TestClassifyError_UnrecognisedDriver_DegradesToOriginalErrorUnchanged(t *testing.T) {
	db, fake := newTestDB()
	plain := errors.New("boom")
	fake.Enqueue(testdb.Result{Err: plain})

	_, err := Of[testWidget](db).Where(widgetCol.ID.Eq(int64(1))).Delete(context.Background())
	require.Error(t, err)
	assert.Same(t, plain, err)
}

// detailingDecoder is a test-local DetailedErrorDecoder standing in for a
// real driver-importing tier-two decoder (e.g. driver/pgxdriver), proving
// the WithErrorDecoder → classifyError → typed-error plumbing carries
// Constraint/Table/Column through end to end (R13.2/R13.4b).
type detailingDecoder struct{}

func (detailingDecoder) Classify(err error) (string, bool) {
	return dialect.CodeUnique, true
}

func (detailingDecoder) Detail(err error) (constraint, table, column string) {
	return "widgets_count_key", "widgets", "count"
}

var _ dialect.DetailedErrorDecoder = detailingDecoder{}

func TestWithErrorDecoder_DetailedDecoder_PopulatesConstraintTableColumn(t *testing.T) {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New(), WithErrorDecoder(detailingDecoder{}))
	fake.Enqueue(testdb.Result{Err: &fakeSQLStateErr{code: "23505"}})

	_, err := Of[testWidget](db).Where(widgetCol.ID.Eq(int64(1))).Delete(context.Background())
	require.Error(t, err)

	v, ok := errors.AsType[*ErrUniqueViolation](err)
	require.True(t, ok)
	assert.Equal(t, "widgets_count_key", v.Constraint)
	assert.Equal(t, "widgets", v.Table)
	assert.Equal(t, "count", v.Column)
}

func TestAutoDetectedDecoder_LeavesConstraintTableColumnEmpty(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Err: &fakeSQLStateErr{code: "23505"}})

	_, err := Of[testWidget](db).Where(widgetCol.ID.Eq(int64(1))).Delete(context.Background())
	require.Error(t, err)

	v, ok := errors.AsType[*ErrUniqueViolation](err)
	require.True(t, ok)
	assert.Empty(t, v.Constraint)
	assert.Empty(t, v.Table)
	assert.Empty(t, v.Column)
}

type fakeSQLiteCodeErr struct{ code int }

func (e *fakeSQLiteCodeErr) Error() string { return "driver error" }
func (e *fakeSQLiteCodeErr) Code() int     { return e.code }

func TestOpen_WithSQLiteDialect_AutoDetectsErrorDecoder(t *testing.T) {
	fake := testdb.New()
	db := Open(fake.Open(), sqlitedialect.New())
	driverErr := &fakeSQLiteCodeErr{code: 2067} // SQLITE_CONSTRAINT_UNIQUE
	fake.Enqueue(testdb.Result{Err: driverErr})

	row := &testWidget{Count: 1}
	err := Create(context.Background(), db, row)
	require.Error(t, err)

	v, ok := errors.AsType[*ErrUniqueViolation](err)
	require.True(t, ok)
	assert.Equal(t, "testWidget", v.Model)
	assert.Equal(t, "Create", v.Operation)
	assert.ErrorIs(t, err, driverErr, "Unwrap must still reach the original driver error")
}
