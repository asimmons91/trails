package schema

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeModel[ID comparable] struct {
	ID ID `db:"id,pk,auto_increment"`
}

func fieldNames(tbl *Table) []string {
	names := make([]string, len(tbl.Fields))
	for i, f := range tbl.Fields {
		names[i] = f.GoName
	}
	return names
}

func columnNames(tbl *Table) []string {
	names := make([]string, len(tbl.Fields))
	for i, f := range tbl.Fields {
		names[i] = f.Column
	}
	return names
}

type valueNamedUser struct {
	fakeModel[int64] `db:"table:ignored_by_method"`
	Email            string `db:"email"`
}

func (valueNamedUser) TableName() string { return "explicit_value" }

func TestBuild_TableName_ValueReceiver_TakesPriorityOverTag(t *testing.T) {
	tbl, err := For(reflect.TypeFor[valueNamedUser]())
	require.NoError(t, err)
	assert.Equal(t, "explicit_value", tbl.Name)
}

type ptrNamedUser struct {
	fakeModel[int64] `db:"table:ignored_by_method"`
	Email            string `db:"email"`
}

func (*ptrNamedUser) TableName() string { return "explicit_ptr" }

func TestBuild_TableName_PointerReceiver_Detected(t *testing.T) {
	tbl, err := For(reflect.TypeFor[ptrNamedUser]())
	require.NoError(t, err)
	assert.Equal(t, "explicit_ptr", tbl.Name)
}

type tableTaggedUser struct {
	fakeModel[int64] `db:"table:users,alias:u"`
	Email            string `db:"email"`
}

func TestBuild_TableName_FromTag_WhenNoMethod(t *testing.T) {
	tbl, err := For(reflect.TypeFor[tableTaggedUser]())
	require.NoError(t, err)
	assert.Equal(t, "users", tbl.Name)
	assert.Equal(t, "u", tbl.Alias)
}

type noTableUser struct {
	fakeModel[int64]
	Email string `db:"email"`
}

func TestBuild_MissingTableName_IsError(t *testing.T) {
	_, err := For(reflect.TypeFor[noTableUser]())
	require.Error(t, err)
	var mErr *ErrMissingTableName
	require.True(t, errors.As(err, &mErr))
}

type noAliasUser struct {
	fakeModel[int64] `db:"table:users_no_alias"`
	Email            string `db:"email"`
}

func TestBuild_Alias_DefaultsToTableName(t *testing.T) {
	tbl, err := For(reflect.TypeFor[noAliasUser]())
	require.NoError(t, err)
	assert.Equal(t, "users_no_alias", tbl.Alias)
}

type withUnexported struct {
	fakeModel[int64] `db:"table:with_unexported"`
	Email            string `db:"email"`
}

func TestBuild_UnexportedField_SkippedWithoutError(t *testing.T) {
	tbl, err := For(reflect.TypeFor[withUnexported]())
	require.NoError(t, err)
	assert.NotContains(t, fieldNames(tbl), "cache")
}

type orderedFields struct {
	fakeModel[int64] `db:"table:ordered_fields"`
	B                string
	A                string
}

func TestBuild_DeclarationOrder_PreservedThroughEmbed(t *testing.T) {
	tbl, err := For(reflect.TypeFor[orderedFields]())
	require.NoError(t, err)
	assert.Equal(t, []string{"ID", "B", "A"}, fieldNames(tbl))
	assert.Equal(t, []string{"id", "b", "a"}, columnNames(tbl))
}

type withIndexPath struct {
	fakeModel[int64] `db:"table:with_index_path"`
	Name             string `db:"name"`
}

func TestBuild_FieldIndex_ResolvesViaFieldByIndex(t *testing.T) {
	tbl, err := For(reflect.TypeFor[withIndexPath]())
	require.NoError(t, err)

	var inst = withIndexPath{ID: 42, Name: "hello"}
	v := reflect.ValueOf(inst)

	for _, f := range tbl.Fields {
		got := v.FieldByIndex(f.Index)
		switch f.Column {
		case "id":
			assert.EqualValues(t, 42, got.Int())
		case "name":
			assert.Equal(t, "hello", got.String())
		default:
			t.Fatalf("unexpected field %q", f.Column)
		}
	}
}

type multipleBadTags struct {
	fakeModel[int64] `db:"table:multiple_bad_tags"`
	A                string `db:"pk"`
	B                string `db:"null_zero"`
}

func TestBuild_MultipleBadTags_AggregatedViaErrorsJoin(t *testing.T) {
	_, err := For(reflect.TypeFor[multipleBadTags]())
	require.Error(t, err)

	joined, ok := err.(interface{ Unwrap() []error })
	require.True(t, ok, "expected errors.Join result, got %T", err)
	errs := joined.Unwrap()
	require.Len(t, errs, 2)

	var a, b *ErrAmbiguousColumnSlot
	require.True(t, errors.As(errs[0], &a) || errors.As(errs[1], &a))
	require.True(t, errors.As(errs[0], &b) || errors.As(errs[1], &b))
}

type simpleMixin struct {
	X int
}

type ptrEmbed struct {
	*simpleMixin
	Name string `db:"name"`
}

func TestBuild_PointerEmbed_IsError(t *testing.T) {
	_, err := For(reflect.TypeFor[ptrEmbed]())
	require.Error(t, err)
	var pErr *ErrPointerEmbed
	require.True(t, errors.As(err, &pErr))
}

type MemberKey struct {
	OrgID  int64
	UserID int64
}

type membership struct {
	fakeModel[MemberKey] `db:"table:memberships"`
	Role                 string `db:"role"`
}

func TestBuild_CompositeKey_ExpandsIntoSubfields(t *testing.T) {
	tbl, err := For(reflect.TypeFor[membership]())
	require.NoError(t, err)

	assert.True(t, tbl.PKIsComposite)
	assert.Equal(t, []string{"org_id", "user_id", "role"}, columnNames(tbl))
	assert.NotContains(t, columnNames(tbl), "id")

	pkCols := make([]string, len(tbl.PK))
	for i, f := range tbl.PK {
		pkCols[i] = f.Column
	}
	assert.ElementsMatch(t, []string{"org_id", "user_id"}, pkCols)
}

type nestedKeyInner struct {
	Nested struct{ X int }
}

type badNestedKey struct {
	fakeModel[nestedKeyInner] `db:"table:bad_nested_key"`
}

func TestBuild_CompositeKey_RejectsNestedStruct(t *testing.T) {
	_, err := For(reflect.TypeFor[badNestedKey]())
	require.Error(t, err)
	var sErr *ErrCompositeKeyShape
	require.True(t, errors.As(err, &sErr))
}

type pointerKeyInner struct {
	P *int
}

type badPointerKey struct {
	fakeModel[pointerKeyInner] `db:"table:bad_pointer_key"`
}

func TestBuild_CompositeKey_RejectsPointerField(t *testing.T) {
	_, err := For(reflect.TypeFor[badPointerKey]())
	require.Error(t, err)
	var sErr *ErrCompositeKeyShape
	require.True(t, errors.As(err, &sErr))
}

type timeKeyed struct {
	fakeModel[time.Time] `db:"table:time_keyed"`
}

func TestBuild_CompositeKey_ExcludesTimeTime(t *testing.T) {
	tbl, err := For(reflect.TypeFor[timeKeyed]())
	require.NoError(t, err)
	assert.False(t, tbl.PKIsComposite)
	assert.Equal(t, []string{"ID"}, fieldNames(tbl))
	assert.Equal(t, []string{"id"}, columnNames(tbl))
}

type myValuerKey struct{ V string }

func (k myValuerKey) Value() (driver.Value, error) { return k.V, nil }

type valuerKeyed struct {
	fakeModel[myValuerKey] `db:"table:valuer_keyed"`
}

func TestBuild_CompositeKey_ExcludesDriverValuer(t *testing.T) {
	tbl, err := For(reflect.TypeFor[valuerKeyed]())
	require.NoError(t, err)
	assert.False(t, tbl.PKIsComposite)
	assert.Equal(t, []string{"id"}, columnNames(tbl))
}

type fakeUUID [16]byte

type uuidKeyed struct {
	fakeModel[fakeUUID] `db:"table:uuid_keyed"`
}

func TestBuild_CompositeKey_ExcludesArrayTypes(t *testing.T) {
	tbl, err := For(reflect.TypeFor[uuidKeyed]())
	require.NoError(t, err)
	assert.False(t, tbl.PKIsComposite)
	assert.Equal(t, []string{"id"}, columnNames(tbl))
}

type Post struct {
	fakeModel[int64] `db:"table:rel_posts"`
	Author           *User `db:"rel:belongs_to"`
}

type User struct {
	fakeModel[int64] `db:"table:rel_users"`
	Posts            []*Post `db:"rel:has_many"`
}

func TestBuild_Relation_BelongsTo_DefaultsFKFromTargetType(t *testing.T) {
	tbl, err := For(reflect.TypeFor[Post]())
	require.NoError(t, err)
	require.Len(t, tbl.Relations, 1)
	rel := tbl.Relations[0]
	assert.Equal(t, BelongsTo, rel.Kind)
	assert.Equal(t, "user_id", rel.FK)
	assert.Equal(t, reflect.TypeFor[User](), rel.TargetType)
	assert.False(t, rel.Slice)
}

func TestBuild_Relation_HasMany_DefaultsFKFromOwnerType(t *testing.T) {
	tbl, err := For(reflect.TypeFor[User]())
	require.NoError(t, err)
	require.Len(t, tbl.Relations, 1)
	rel := tbl.Relations[0]
	assert.Equal(t, HasMany, rel.Kind)
	assert.Equal(t, "user_id", rel.FK)
	assert.Equal(t, reflect.TypeFor[Post](), rel.TargetType)
	assert.True(t, rel.Slice)
}

type PostExplicitFK struct {
	fakeModel[int64] `db:"table:rel_explicit_fk"`
	Author           *User `db:"rel:belongs_to,fk:author_id"`
}

func TestBuild_Relation_ExplicitFK_NotOverridden(t *testing.T) {
	tbl, err := For(reflect.TypeFor[PostExplicitFK]())
	require.NoError(t, err)
	require.Len(t, tbl.Relations, 1)
	assert.Equal(t, "author_id", tbl.Relations[0].FK)
}

type badRelationShape struct {
	fakeModel[int64] `db:"table:bad_relation_shape"`
	Author           User `db:"rel:belongs_to"`
}

func TestBuild_Relation_WrongShape_IsError(t *testing.T) {
	_, err := For(reflect.TypeFor[badRelationShape]())
	require.Error(t, err)
	var rErr *ErrRelationFieldShape
	require.True(t, errors.As(err, &rErr))
}

type concurrentModel struct {
	fakeModel[int64] `db:"table:concurrent_model"`
	Name             string `db:"name"`
}

func TestFor_ConcurrentCalls_BuildsOnce(t *testing.T) {
	typ := reflect.TypeFor[concurrentModel]()

	const n = 50
	results := make([]*Table, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = For(typ)
		}(i)
	}
	wg.Wait()

	for i := range n {
		require.NoError(t, errs[i])
		require.Same(t, results[0], results[i])
	}
}

type unhookedModel struct {
	fakeModel[int64] `db:"table:unhooked_models"`
	Name             string `db:"name"`
}

func TestBuild_NoHooks_AllFalse(t *testing.T) {
	tbl, err := For(reflect.TypeFor[unhookedModel]())
	require.NoError(t, err)
	assert.Equal(t, Hooks{}, tbl.Hooks)
}

type fullyHookedModel struct {
	fakeModel[int64] `db:"table:fully_hooked_models"`
	Name             string `db:"name"`
}

func (*fullyHookedModel) BeforeInsert(ctx context.Context) error { return nil }
func (*fullyHookedModel) AfterInsert(ctx context.Context) error  { return nil }
func (*fullyHookedModel) BeforeUpdate(ctx context.Context) error { return nil }
func (*fullyHookedModel) AfterUpdate(ctx context.Context) error  { return nil }
func (*fullyHookedModel) BeforeDelete(ctx context.Context) error { return nil }
func (*fullyHookedModel) AfterDelete(ctx context.Context) error  { return nil }
func (*fullyHookedModel) AfterScan(ctx context.Context) error    { return nil }

func TestBuild_AllHooksPointerReceiver_AllDetected(t *testing.T) {
	tbl, err := For(reflect.TypeFor[fullyHookedModel]())
	require.NoError(t, err)
	assert.Equal(t, Hooks{
		BeforeInsert: true, AfterInsert: true,
		BeforeUpdate: true, AfterUpdate: true,
		BeforeDelete: true, AfterDelete: true,
		AfterScan: true,
	}, tbl.Hooks)
}

type partiallyHookedModel struct {
	fakeModel[int64] `db:"table:partially_hooked_models"`
	Name             string `db:"name"`
}

func (*partiallyHookedModel) AfterScan(ctx context.Context) error { return nil }

func TestBuild_OneHook_OnlyThatOneDetected(t *testing.T) {
	tbl, err := For(reflect.TypeFor[partiallyHookedModel]())
	require.NoError(t, err)
	assert.Equal(t, Hooks{AfterScan: true}, tbl.Hooks)
}

type valueReceiverHookModel struct {
	fakeModel[int64] `db:"table:value_receiver_hook_models"`
	Name             string `db:"name"`
}

func (valueReceiverHookModel) BeforeInsert(ctx context.Context) error { return nil }

func TestBuild_ValueReceiverHook_IsError(t *testing.T) {
	_, err := For(reflect.TypeFor[valueReceiverHookModel]())
	require.Error(t, err)
	var vErr *ErrValueReceiverHook
	require.True(t, errors.As(err, &vErr))
	assert.Equal(t, "valueReceiverHookModel", vErr.Struct)
	assert.Equal(t, "BeforeInsert", vErr.Method)
}
