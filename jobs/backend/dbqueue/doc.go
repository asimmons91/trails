// Package dbqueue implements a jobs.Backend backed by a SQL table via pack:
// Enqueue persists each job as a row, and Run polls for available rows,
// dispatches them to a jobs.Registry, and retries failures with backoff
// until MaxAttempts is exhausted.
//
// Run must be started separately from New — typically as a trails.Runner
// alongside the HTTP server — since New only builds the Backend; nothing is
// processed until Run is polling. Apply Migration against the app's
// database before using a Backend for the first time.
//
// Claiming a row is done with either `SELECT ... FOR UPDATE SKIP LOCKED`
// (claimOneLocking) or, on SQLite (which has no row locking), a single
// `UPDATE ... RETURNING` statement (claimOneSQLite) — whichever the
// dialect supports — so concurrent workers, in-process or across
// processes, never claim the same row twice. Enqueued's ConcurrencyKey/
// ConcurrencyLimit/ConcurrencyDuration are enforced by the same claim
// query, via a correlated subquery counting other rows currently executing
// under the same key within the window.
package dbqueue
