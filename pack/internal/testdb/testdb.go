// Package testdb is a queue-driven fake *sql.DB for testing pack without a
// real database. FakeDB.Enqueue primes canned Results that Open's
// connection dequeues FIFO on each Exec/Query, while Executed, TxEvents,
// and RowsClosedCount record what actually happened for a test to assert
// against.
package testdb

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
)

// Recorded is one captured Exec/Query call.
type Recorded struct {
	SQL  string
	Args []any
}

// TxEvent is one captured transaction lifecycle event: "BEGIN" (with
// Isolation/ReadOnly reflecting the *sql.TxOptions passed through),
// "COMMIT", or "ROLLBACK".
type TxEvent struct {
	Kind      string // "BEGIN", "COMMIT", "ROLLBACK"
	Isolation driver.IsolationLevel
	ReadOnly  bool
}

// Result is one canned response to be replayed for a single Exec or Query
// call, dequeued in FIFO order.
type Result struct {
	Columns      []string
	Rows         [][]driver.Value
	LastInsertID int64
	RowsAffected int64
	Err          error
}

// Row converts ordinary Go values into a []driver.Value row for use in
// Result.Rows. It panics at test-setup time (not mid-test) if a value's
// type is not one driver.Value accepts.
func Row(vals ...any) []driver.Value {
	row := make([]driver.Value, len(vals))
	for i, v := range vals {
		if v == nil {
			row[i] = nil
			continue
		}
		dv, err := driver.DefaultParameterConverter.ConvertValue(v)
		if err != nil {
			panic("testdb.Row: value " + "at index " + itoa(i) + " is not a valid driver.Value: " + err.Error())
		}
		row[i] = dv
	}
	return row
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// FakeDB is a queue-driven fake database. Each Exec/Query executed against
// a *sql.DB opened via Open dequeues the next Result.
type FakeDB struct {
	mu         sync.Mutex
	executed   []Recorded
	queue      []Result
	txEvents   []TxEvent
	rowsClosed int
}

// New returns an empty FakeDB.
func New() *FakeDB {
	return &FakeDB{}
}

// Open returns a *sql.DB backed by this FakeDB.
func (f *FakeDB) Open() *sql.DB {
	return sql.OpenDB(&fakeConnector{db: f})
}

// Enqueue appends r to the FIFO result queue and returns f for chaining.
func (f *FakeDB) Enqueue(r Result) *FakeDB {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, r)
	return f
}

// Executed returns a defensive copy of every (sql, args) call recorded so far.
func (f *FakeDB) Executed() []Recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Recorded, len(f.executed))
	copy(out, f.executed)
	return out
}

// Reset clears both the executed history and the pending result queue.
func (f *FakeDB) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = nil
	f.queue = nil
	f.txEvents = nil
	f.rowsClosed = 0
}

// TxEvents returns a defensive copy of every BEGIN/COMMIT/ROLLBACK recorded
// so far, in order.
func (f *FakeDB) TxEvents() []TxEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]TxEvent, len(f.txEvents))
	copy(out, f.txEvents)
	return out
}

// RowsClosedCount returns how many times a *sql.Rows returned by this
// FakeDB has had Close called on it.
func (f *FakeDB) RowsClosedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rowsClosed
}

func (f *FakeDB) recordTxBegin(opts driver.TxOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txEvents = append(f.txEvents, TxEvent{
		Kind:      "BEGIN",
		Isolation: opts.Isolation,
		ReadOnly:  opts.ReadOnly,
	})
}

func (f *FakeDB) recordTxEnd(kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.txEvents = append(f.txEvents, TxEvent{Kind: kind})
}

func (f *FakeDB) recordRowsClosed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rowsClosed++
}

var errNoResultQueued = errors.New("testdb: no result queued for this statement")

func (f *FakeDB) record(sql string, args []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = append(f.executed, Recorded{SQL: sql, Args: args})
}

func (f *FakeDB) next() (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queue) == 0 {
		return Result{}, errNoResultQueued
	}
	r := f.queue[0]
	f.queue = f.queue[1:]
	return r, nil
}
