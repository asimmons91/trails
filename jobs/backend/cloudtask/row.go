package cloudtask

import "github.com/asimmons91/trails/pack"

const (
	statePending   = "pending"
	stateExecuting = "executing"
	stateFinished  = "finished"
	stateFailed    = "failed"
)

const defaultQueue = "default"

type jobRow struct {
	pack.Model[int64] `db:"table:cloudtask_jobs"`

	Kind                  string `db:"kind,not_null"`
	Args                  string `db:"args,not_null"`
	Queue                 string `db:"queue,not_null"`
	State                 string `db:"state,not_null"`
	Attempts              int    `db:"attempts,not_null"`
	MaxAttempts           int    `db:"max_attempts,not_null"`
	ConcurrencyKey        string `db:"concurrency_key,not_null"`
	ConcurrencyLimit      int    `db:"concurrency_limit,not_null"`
	ConcurrencyDurationNs int64  `db:"concurrency_duration_ns,not_null"`
	LockedAt              int64  `db:"locked_at,not_null"`
	TaskName              string `db:"task_name,not_null"` // Cloud Tasks resource name, for debugging/correlation
	LastError             string `db:"last_error,not_null"`
	FinishedAt            int64  `db:"finished_at,not_null"`
	CreatedAt             int64  `db:"created_at,not_null"`
}

var jobCol = struct {
	ID                    pack.Col[jobRow, int64]
	Kind                  pack.Col[jobRow, string]
	Args                  pack.Col[jobRow, string]
	Queue                 pack.Col[jobRow, string]
	State                 pack.Col[jobRow, string]
	Attempts              pack.Col[jobRow, int]
	MaxAttempts           pack.Col[jobRow, int]
	ConcurrencyKey        pack.Col[jobRow, string]
	ConcurrencyLimit      pack.Col[jobRow, int]
	ConcurrencyDurationNs pack.Col[jobRow, int64]
	LockedAt              pack.Col[jobRow, int64]
	TaskName              pack.Col[jobRow, string]
	LastError             pack.Col[jobRow, string]
	FinishedAt            pack.Col[jobRow, int64]
	CreatedAt             pack.Col[jobRow, int64]
}{
	ID:                    pack.Field(func(r *jobRow) *int64 { return &r.ID }),
	Kind:                  pack.Field(func(r *jobRow) *string { return &r.Kind }),
	Args:                  pack.Field(func(r *jobRow) *string { return &r.Args }),
	Queue:                 pack.Field(func(r *jobRow) *string { return &r.Queue }),
	State:                 pack.Field(func(r *jobRow) *string { return &r.State }),
	Attempts:              pack.Field(func(r *jobRow) *int { return &r.Attempts }),
	MaxAttempts:           pack.Field(func(r *jobRow) *int { return &r.MaxAttempts }),
	ConcurrencyKey:        pack.Field(func(r *jobRow) *string { return &r.ConcurrencyKey }),
	ConcurrencyLimit:      pack.Field(func(r *jobRow) *int { return &r.ConcurrencyLimit }),
	ConcurrencyDurationNs: pack.Field(func(r *jobRow) *int64 { return &r.ConcurrencyDurationNs }),
	LockedAt:              pack.Field(func(r *jobRow) *int64 { return &r.LockedAt }),
	TaskName:              pack.Field(func(r *jobRow) *string { return &r.TaskName }),
	LastError:             pack.Field(func(r *jobRow) *string { return &r.LastError }),
	FinishedAt:            pack.Field(func(r *jobRow) *int64 { return &r.FinishedAt }),
	CreatedAt:             pack.Field(func(r *jobRow) *int64 { return &r.CreatedAt }),
}
