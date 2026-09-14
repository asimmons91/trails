package pack

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingQueryHook struct {
	before []QueryEvent // snapshot at BeforeQuery time
	after  []QueryEvent // snapshot at AfterQuery time
}

func (h *recordingQueryHook) BeforeQuery(ctx context.Context, ev *QueryEvent) context.Context {
	h.before = append(h.before, *ev)
	return ctx
}

func (h *recordingQueryHook) AfterQuery(ctx context.Context, ev *QueryEvent) {
	h.after = append(h.after, *ev)
}

func TestQueryHook_BeforeAndAfterQuery_Fire(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)

	require.Len(t, hook.before, 1)
	require.Len(t, hook.after, 1)
	assert.Equal(t, "Find", hook.before[0].Operation)
	assert.Equal(t, "testUser", hook.before[0].Model)
	assert.Equal(t, fake.Executed()[0].SQL, hook.before[0].SQL)
	assert.Equal(t, "Find", hook.after[0].Operation)
	assert.NoError(t, hook.after[0].Err)
}

func TestQueryHook_ArgsHiddenByDefault(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Find(context.Background())
	require.NoError(t, err)

	require.Len(t, hook.before, 1)
	assert.Nil(t, hook.before[0].Args)
}

func TestQueryHook_WithQueryHookArgs_PopulatesArgs(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook), WithQueryHookArgs())
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Find(context.Background())
	require.NoError(t, err)

	require.Len(t, hook.before, 1)
	assert.Equal(t, []any{18}, hook.before[0].Args)
}

func TestQueryHook_MultipleHooks_FireInRegistrationOrder(t *testing.T) {
	fake := testdb.New()
	var order []string
	hookA := &orderRecordingHook{name: "A", order: &order}
	hookB := &orderRecordingHook{name: "B", order: &order}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hookA), WithQueryHook(hookB))
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"A-before", "B-before", "A-after", "B-after"}, order)
}

type orderRecordingHook struct {
	name  string
	order *[]string
}

func (h *orderRecordingHook) BeforeQuery(ctx context.Context, ev *QueryEvent) context.Context {
	*h.order = append(*h.order, h.name+"-before")
	return ctx
}

func (h *orderRecordingHook) AfterQuery(ctx context.Context, ev *QueryEvent) {
	*h.order = append(*h.order, h.name+"-after")
}

func TestQueryHook_AfterQuery_FiresEvenOnError(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	boom := errors.New("boom")
	fake.Enqueue(testdb.Result{Err: boom})

	_, err := Of[testUser](db).Find(context.Background())
	require.Error(t, err)

	require.Len(t, hook.before, 1)
	require.Len(t, hook.after, 1)
	assert.ErrorIs(t, hook.after[0].Err, boom)
}

func TestQueryHook_ExecOperation_RecordsRowsAffected(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{RowsAffected: 3})

	row := &testAccount{Model: Model[int64]{ID: 1}, Email: "a@b.com"}
	err := Update(context.Background(), db, row)
	require.NoError(t, err)

	require.Len(t, hook.after, 1)
	assert.Equal(t, "Update", hook.after[0].Operation)
	assert.EqualValues(t, 3, hook.after[0].RowsAffected)
}

func TestQueryHook_Preload_IsObserved(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{Columns: []string{"id", "user_id", "title"}})

	_, err := Of[testAccount](db).Preload(accountRel.Posts).Find(context.Background())
	require.NoError(t, err)

	require.Len(t, hook.before, 2)
	assert.Equal(t, "Find", hook.before[0].Operation)
	assert.Equal(t, "Preload", hook.before[1].Operation)
	assert.Equal(t, "testPost", hook.before[1].Model)
}
