// Package fakedb is a minimal in-memory database/sql driver for tests.
//
// It lets the generator's introspector, database factory, and repository
// tests exercise the real database/sql code paths (including
// QueryContext/QueryRowContext/Tx) without standing up a Postgres instance.
//
// The driver is registered under the name "fakedb" in package init. Tests
// open it with sql.Open("fakedb", dsn) where the dsn is a key into the
// package's table of fake databases (registered via Register).
//
// Since 1.4.0.
package fakedb

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Register a fakeDB under the given name. Returns the db so the test can
// populate it with QueryResult / ExecResult before opening. Since 1.4.0.
func Register(name string) *DB {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := dbs[name]; ok {
		panic("fakedb: duplicate registration: " + name)
	}
	d := &DB{name: name}
	dbs[name] = d
	if driverRegistered == false {
		sql.Register("fakedb", &driverImpl{})
		driverRegistered = true
	}
	return d
}

// Reset removes all registered fakeDBs (call between test groups). Since 1.4.0.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	dbs = map[string]*DB{}
}

var (
	mu               sync.Mutex
	dbs              = map[string]*DB{}
	driverRegistered = false
)

// DB is one fake database. Since 1.4.0.
type DB struct {
	name string

	qmu       sync.Mutex
	queries   []queryEntry // ordered; first match wins
	execCalls []ExecCall
}

// QueryCall records one Query call the driver received. Since 1.4.0.
type QueryCall struct {
	Query string
	Args  []driver.Value
}

// ExecCall records one Exec call the driver received. Since 1.4.0.
type ExecCall struct {
	Query string
	Args  []driver.Value
}

// Rows is a canned result set. Since 1.4.0.
type Rows struct {
	Cols   []string
	Values [][]any // row-major; each row is a slice of column values
}

// queryEntry pairs a predicate with a canned result. Since 1.4.0.
type queryEntry struct {
	match   func(query string) bool
	query   string // for inspection
	rows    *Rows
	execErr error
	lastArgs []driver.Value // most recent args passed for this entry
}

// recordArgs stores the args for the first entry matching the query. Since 1.4.0.
func (d *DB) recordArgs(query string, args []driver.Value) {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	for i := range d.queries {
		if d.queries[i].match(query) {
			d.queries[i].lastArgs = append([]driver.Value(nil), args...)
			return
		}
	}
}

// AddRows registers a canned result for the next Query whose SQL contains
// the given substring. First match wins, in registration order. Since 1.4.0.
func (d *DB) AddRows(contains string, rows *Rows) {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	d.queries = append(d.queries, queryEntry{
		match: func(q string) bool { return strings.Contains(q, contains) },
		query: contains,
		rows:  rows,
	})
}

// AddError registers a canned error for the next Query whose SQL contains
// the given substring. Since 1.4.0.
func (d *DB) AddError(contains string, err error) {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	d.queries = append(d.queries, queryEntry{
		match:   func(q string) bool { return strings.Contains(q, contains) },
		query:   contains,
		execErr: err,
	})
}

// ExecCalls returns the recorded Exec calls (for assertion). Since 1.4.0.
func (d *DB) ExecCalls() []ExecCall {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	return append([]ExecCall(nil), d.execCalls...)
}

// Name returns the registration name (used as the DSN for sql.Open). Since 1.4.0.
func (d *DB) Name() string { return d.name }

// LastQueryArgs returns the args of the most recent Query matching the
// substring. Since 1.4.0.
func (d *DB) LastQueryArgs(contains string) []driver.Value {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	for i := len(d.queries) - 1; i >= 0; i-- {
		if strings.Contains(d.queries[i].query, contains) {
			return d.queries[i].lastArgs
		}
	}
	return nil
}

// match finds the canned entry for a query. Since 1.4.0.
func (d *DB) match(query string) (queryEntry, bool) {
	d.qmu.Lock()
	defer d.qmu.Unlock()
	for _, e := range d.queries {
		if e.match(query) {
			return e, true
		}
	}
	return queryEntry{}, false
}

// ---- driver.Driver --------------------------------------------------------

type driverImpl struct{}

func (d *driverImpl) Open(dsn string) (driver.Conn, error) {
	mu.Lock()
	db, ok := dbs[dsn]
	mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fakedb: no database registered for dsn %q", dsn)
	}
	return &conn{db: db}, nil
}

// ---- driver.Conn ----------------------------------------------------------

type conn struct{ db *DB }

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return &stmt{db: c.db, query: query}, nil
}
func (c *conn) Close() error                                   { return nil }
func (c *conn) Begin() (driver.Tx, error)                     { return &tx{}, nil }

// ---- driver.Tx ------------------------------------------------------------

type tx struct{}

func (t *tx) Commit() error   { return nil }
func (t *tx) Rollback() error { return nil }

// ---- driver.Stmt ----------------------------------------------------------

type stmt struct {
	db    *DB
	query string
}

func (s *stmt) Close() error        { return nil }
func (s *stmt) NumInput() int       { return -1 } // variadic
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	s.db.recordArgs(s.query, args)
	s.db.qmu.Lock()
	s.db.execCalls = append(s.db.execCalls, ExecCall{Query: s.query, Args: append([]driver.Value(nil), args...)})
	s.db.qmu.Unlock()
	// If a canned error was registered for this query, return it (e.g. to
	// simulate a duplicate-key violation).
	if entry, ok := s.db.match(s.query); ok && entry.execErr != nil {
		return nil, entry.execErr
	}
	return &result{}, nil
}
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	entry, ok := s.db.match(s.query)
	if !ok {
		return nil, fmt.Errorf("fakedb: no canned result for query %q", s.query)
	}
	s.db.recordArgs(s.query, args)
	if entry.execErr != nil {
		return nil, entry.execErr
	}
	if entry.rows == nil {
		return nil, errors.New("fakedb: canned entry has no rows")
	}
	return &rowsImpl{cols: entry.rows.Cols, values: entry.rows.Values, idx: -1}, nil
}

// ---- driver.Rows ----------------------------------------------------------

type rowsImpl struct {
	cols   []string
	values [][]any
	idx    int
}

func (r *rowsImpl) Columns() []string           { return r.cols }
func (r *rowsImpl) Close() error                { return nil }
func (r *rowsImpl) Next(dest []driver.Value) error {
	r.idx++
	if r.idx >= len(r.values) {
		return io.EOF // database/sql convention: io.EOF signals end of rows
	}
	row := r.values[r.idx]
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}

// ---- driver.Result --------------------------------------------------------

type result struct{}

func (r *result) LastInsertId() (int64, error) { return 0, nil }
func (r *result) RowsAffected() (int64, error) { return 1, nil }
