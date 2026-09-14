package pack

import (
	"context"
	"fmt"
	"time"
)

type testUser struct {
	ID    int64  `db:"id,pk"`
	Email string `db:"email"`
	Age   int    `db:"age"`
}

func (testUser) TableName() string { return "users" }

var userCol = struct {
	ID    Col[testUser, int64]
	Email Col[testUser, string]
	Age   Col[testUser, int]
}{
	ID:    Field(func(u *testUser) *int64 { return &u.ID }),
	Email: Field(func(u *testUser) *string { return &u.Email }),
	Age:   Field(func(u *testUser) *int { return &u.Age }),
}

type addressMixin struct {
	City string `db:"city"`
}

type testCustomer struct {
	ID int64 `db:"id,pk"`
	addressMixin
	Name string `db:"name"`
}

func (testCustomer) TableName() string { return "customers" }

var customerCol = struct {
	ID   Col[testCustomer, int64]
	City Col[testCustomer, string]
	Name Col[testCustomer, string]
}{
	ID:   Field(func(c *testCustomer) *int64 { return &c.ID }),
	City: Field(func(c *testCustomer) *string { return &c.City }),
	Name: Field(func(c *testCustomer) *string { return &c.Name }),
}

// testMemberKey is R5.10's own worked composite-key example.
type testMemberKey struct {
	OrgID  int64 `db:"org_id"`
	UserID int64 `db:"user_id"`
}

type testMembership struct {
	Model[testMemberKey] `db:"table:memberships"`
	Role                 string      `db:"role"`
	Notes                []*testPost `db:"rel:has_many,fk:user_id"`
}

var membershipRel = struct {
	Notes Rel[testMembership, testPost]
}{
	Notes: Relation(func(m *testMembership) *[]*testPost { return &m.Notes }),
}

type testAccount struct {
	Model[int64] `db:"table:accounts"`
	Email        string       `db:"email"`
	CreatedAt    time.Time    `db:",default:now()"`
	Nickname     string       `db:",null_zero"`
	Posts        []*testPost  `db:"rel:has_many,fk:user_id"`
	Profile      *testProfile `db:"rel:has_one,fk:account_id"`
}

var accountCol = struct {
	ID    Col[testAccount, int64]
	Email Col[testAccount, string]
}{
	ID:    Field(func(a *testAccount) *int64 { return &a.ID }),
	Email: Field(func(a *testAccount) *string { return &a.Email }),
}

var accountRel = struct {
	Posts   Rel[testAccount, testPost]
	Profile Rel[testAccount, testProfile]
}{
	Posts:   Relation(func(a *testAccount) *[]*testPost { return &a.Posts }),
	Profile: RelationOne(func(a *testAccount) **testProfile { return &a.Profile }),
}

type testPost struct {
	Model[int64] `db:"table:posts"`
	UserID       int64        `db:"user_id"`
	Title        string       `db:"title"`
	Author       *testAccount `db:"rel:belongs_to,fk:user_id"`
}

var postCol = struct {
	ID     Col[testPost, int64]
	UserID Col[testPost, int64]
	Title  Col[testPost, string]
}{
	ID:     Field(func(p *testPost) *int64 { return &p.ID }),
	UserID: Field(func(p *testPost) *int64 { return &p.UserID }),
	Title:  Field(func(p *testPost) *string { return &p.Title }),
}

var postRel = struct {
	Author Rel[testPost, testAccount]
}{
	Author: RelationOne(func(p *testPost) **testAccount { return &p.Author }),
}

type testProfile struct {
	Model[int64] `db:"table:profiles"`
	AccountID    int64  `db:"account_id"`
	Bio          string `db:"bio"`
}

var profileCol = struct {
	AccountID Col[testProfile, int64]
	Bio       Col[testProfile, string]
}{
	AccountID: Field(func(p *testProfile) *int64 { return &p.AccountID }),
	Bio:       Field(func(p *testProfile) *string { return &p.Bio }),
}

type testAccountSummary struct {
	Email string `db:"email"`
}

func (testAccountSummary) TableName() string { return "accounts" }

// testWidget isolates a uint64 field so the R12.1a overflow golden SQL
// doesn't entangle with other columns.
type testWidget struct {
	Model[int64] `db:"table:widgets"`
	Count        uint64 `db:"count"`
}

var widgetCol = struct {
	ID    Col[testWidget, int64]
	Count Col[testWidget, uint64]
}{
	ID:    Field(func(w *testWidget) *int64 { return &w.ID }),
	Count: Field(func(w *testWidget) *uint64 { return &w.Count }),
}

// --- Hooked fixture (M5) ---

// hookRecord is one observed model-hook invocation, appended to hookLog by
// testHookedItem's own hook methods so tests can assert firing order.
type hookRecord struct {
	Op string
	ID int64
}

var hookLog []hookRecord

func resetHookLog() { hookLog = nil }

type testHookedItem struct {
	Model[int64] `db:"table:hooked_items"`
	Name         string `db:"name"`
	FailHook     string `db:"-"`
}

func (h *testHookedItem) failIfNamed(op string) error {
	if h.FailHook == op {
		return fmt.Errorf("boom from %s", op)
	}
	return nil
}

func (h *testHookedItem) BeforeInsert(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "BeforeInsert", ID: h.ID})
	return h.failIfNamed("BeforeInsert")
}

func (h *testHookedItem) AfterInsert(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "AfterInsert", ID: h.ID})
	return h.failIfNamed("AfterInsert")
}

func (h *testHookedItem) BeforeUpdate(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "BeforeUpdate", ID: h.ID})
	return h.failIfNamed("BeforeUpdate")
}

func (h *testHookedItem) AfterUpdate(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "AfterUpdate", ID: h.ID})
	return h.failIfNamed("AfterUpdate")
}

func (h *testHookedItem) BeforeDelete(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "BeforeDelete", ID: h.ID})
	return h.failIfNamed("BeforeDelete")
}

func (h *testHookedItem) AfterDelete(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "AfterDelete", ID: h.ID})
	return h.failIfNamed("AfterDelete")
}

func (h *testHookedItem) AfterScan(ctx context.Context) error {
	hookLog = append(hookLog, hookRecord{Op: "AfterScan", ID: h.ID})
	return h.failIfNamed("AfterScan")
}

var hookedItemCol = struct {
	ID   Col[testHookedItem, int64]
	Name Col[testHookedItem, string]
}{
	ID:   Field(func(h *testHookedItem) *int64 { return &h.ID }),
	Name: Field(func(h *testHookedItem) *string { return &h.Name }),
}
