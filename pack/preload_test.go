package pack

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreload_HasMany_BatchesParentKeys(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a@b.com", time.Time{}, ""),
			testdb.Row(int64(2), "c@d.com", time.Time{}, ""),
		},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows: [][]driver.Value{
			testdb.Row(int64(10), int64(1), "post1"),
			testdb.Row(int64(11), int64(1), "post2"),
			testdb.Row(int64(12), int64(2), "post3"),
		},
	})

	got, err := Of[testAccount](db).Preload(accountRel.Posts).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t,
		`SELECT "posts"."id", "posts"."user_id", "posts"."title" FROM "posts" AS "posts" WHERE "posts"."user_id" IN ($1, $2)`,
		executed[1].SQL)
	assert.Equal(t, []any{int64(1), int64(2)}, executed[1].Args)

	require.Len(t, got, 2)
	require.Len(t, got[0].Posts, 2)
	assert.Equal(t, "post1", got[0].Posts[0].Title)
	assert.Equal(t, "post2", got[0].Posts[1].Title)
	require.Len(t, got[1].Posts, 1)
	assert.Equal(t, "post3", got[1].Posts[0].Title)
}

// TestPreload_HasOne_DefaultsRefToOwnersPK proves the has_one direction of
// R9.2's Ref default: the FK lives on the target (profiles.account_id),
// and Ref defaults to the OWNER's own PK, not the target's.
func TestPreload_HasOne_DefaultsRefToOwnersPK(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "account_id", "bio"},
		Rows:    [][]driver.Value{testdb.Row(int64(100), int64(1), "hi")},
	})

	got, err := Of[testAccount](db).Preload(accountRel.Profile).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t,
		`SELECT "profiles"."id", "profiles"."account_id", "profiles"."bio" FROM "profiles" AS "profiles" WHERE "profiles"."account_id" IN ($1)`,
		executed[1].SQL)
	assert.Equal(t, []any{int64(1)}, executed[1].Args)

	require.Len(t, got, 1)
	require.NotNil(t, got[0].Profile)
	assert.Equal(t, "hi", got[0].Profile.Bio)
}

// TestPreload_BelongsTo_DefaultsRefToTargetsPK proves the belongs_to
// direction: the FK lives on the owner (posts.user_id), and Ref defaults
// to the TARGET's own PK.
func TestPreload_BelongsTo_DefaultsRefToTargetsPK(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows:    [][]driver.Value{testdb.Row(int64(10), int64(1), "post1")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})

	got, err := Of[testPost](db).Preload(postRel.Author).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t,
		`SELECT "accounts"."id", "accounts"."email", "accounts"."created_at", "accounts"."nickname" FROM "accounts" AS "accounts" WHERE "accounts"."id" IN ($1)`,
		executed[1].SQL)
	assert.Equal(t, []any{int64(1)}, executed[1].Args)

	require.Len(t, got, 1)
	require.NotNil(t, got[0].Author)
	assert.Equal(t, "a@b.com", got[0].Author.Email)
}

func TestPreload_SiblingPreloads_ExactlyThreeQueries(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows:    [][]driver.Value{testdb.Row(int64(10), int64(1), "post1")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "account_id", "bio"},
		Rows:    [][]driver.Value{testdb.Row(int64(100), int64(1), "hi")},
	})

	got, err := Of[testAccount](db).
		Preload(accountRel.Posts).
		Preload(accountRel.Profile).
		Find(context.Background())
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 3)
	require.Len(t, got, 1)
	require.Len(t, got[0].Posts, 1)
	require.NotNil(t, got[0].Profile)
}

// TestPreload_Nested_ExactlyThreeQueries: the nested preload's batched
// query runs off the scanned CHILD rows (posts), not the original parents.
func TestPreload_Nested_ExactlyThreeQueries(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows:    [][]driver.Value{testdb.Row(int64(10), int64(1), "post1")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})

	got, err := Of[testAccount](db).
		Preload(accountRel.Posts, Preload(postRel.Author)).
		Find(context.Background())
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 3)
	require.Len(t, got, 1)
	require.Len(t, got[0].Posts, 1)
	require.NotNil(t, got[0].Posts[0].Author)
	assert.Equal(t, "a@b.com", got[0].Posts[0].Author.Email)
}

func TestPreload_ConstrainedViaWhere(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows:    [][]driver.Value{testdb.Row(int64(10), int64(1), "post1")},
	})

	_, err := Of[testAccount](db).
		Preload(accountRel.Posts, Where(postCol.Title.Eq("post1"))).
		Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t,
		`SELECT "posts"."id", "posts"."user_id", "posts"."title" FROM "posts" AS "posts" WHERE "posts"."user_id" IN ($1) AND "posts"."title" = $2`,
		executed[1].SQL)
	assert.Equal(t, []any{int64(1), "post1"}, executed[1].Args)
}

func TestPreload_HasMany_NoMatch_LeavesNilSlice(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{Columns: []string{"id", "user_id", "title"}})

	got, err := Of[testAccount](db).Preload(accountRel.Posts).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Posts)
}

func TestPreload_HasOne_NoMatch_LeavesNilPointer(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", time.Time{}, "")},
	})
	fake.Enqueue(testdb.Result{Columns: []string{"id", "account_id", "bio"}})

	got, err := Of[testAccount](db).Preload(accountRel.Profile).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Profile)
}

func TestPreload_ZeroParentRows_IssuesNoQuery(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "created_at", "nickname"}})

	got, err := Of[testAccount](db).Preload(accountRel.Posts).Find(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Len(t, fake.Executed(), 1)
}

func TestPreload_RefDefaultAgainstCompositeKey_ReturnsError(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"org_id", "user_id", "role"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), int64(2), "admin")},
	})

	_, err := Of[testMembership](db).Preload(membershipRel.Notes).Find(context.Background())
	require.Error(t, err)
	require.EqualError(t, err,
		"pack: relation Notes: cannot default ref against a composite or missing primary key on testMembership; set ref: explicitly")
}
