// Package databasefactory provides the connection pool, migration runner, and
// transaction wrapper used by the generator (for introspection) and mirrored
// in the generated-api.
//
// v1.4.0 hardening (P0-3.1 + log-before-rethrow):
//   - Tx has an AppError pass-through: if the body returns an AppError, Tx
//     returns it unchanged so the caller's typed-error mapping is not
//     swallowed by a generic catch. (The original P0-3.1 bug: Tx caught the
//     raw driver error first, so UserRepository.Save's duplicate-key →
//     API-003 mapping was dead code.)
//   - Init, Migrate, and Tx each log the underlying cause at ERROR (with
//     stack-friendly attributes) before re-throwing as a typed
//     DatabaseError (GEN-002).
//
// Every public function is wrapped by AppLogger.Logged.
//
// Since 1.4.0.
package databasefactory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"

	// Postgres driver.
	_ "github.com/lib/pq"
)

// DriverName is the registered SQL driver name. Since 1.4.0.
const DriverName = "postgres"

// Factory owns the *sql.DB and exposes Init/Migrate/Tx. Since 1.4.0.
type Factory struct {
	log *applogger.Logger
	db  *sql.DB
}

// New constructs a Factory without opening the DB. Since 1.4.0.
func New(log *applogger.Logger) *Factory {
	if log == nil {
		log = applogger.New()
	}
	return &Factory{log: log}
}

// Init opens the pool and pings it. The driverName selects the registered
// database/sql driver (production: "postgres"; tests: "fakedb"). Returns
// GEN-002 on failure. Since 1.4.0.
func (f *Factory) Init(ctx context.Context, driverName, dsn string) error {
	return f.log.Logged("DatabaseFactory.Init", func() error {
		if driverName == "" {
			driverName = DriverName
		}
		db, err := sql.Open(driverName, dsn)
		if err != nil {
			// v1.4.0: log the cause before rethrowing.
			f.log.Error("DatabaseFactory.Init: sql.Open failed", "driver", driverName, "err", err)
			return errors.Wrap(errors.CodeDatabase, "sql.Open failed", err)
		}
		if err := db.PingContext(ctx); err != nil {
			f.log.Error("DatabaseFactory.Init: ping failed", "err", err)
			_ = db.Close()
			return errors.Wrap(errors.CodeDatabase, "ping failed", err)
		}
		f.db = db
		return nil
	})
}

// DB returns the underlying *sql.DB. Panics if Init was not called; this is
// a programmer error, not a runtime one. Since 1.4.0.
func (f *Factory) DB() *sql.DB {
	if f.db == nil {
		panic("DatabaseFactory: Init not called")
	}
	return f.db
}

// Close closes the pool. Since 1.4.0.
func (f *Factory) Close() error {
	return f.log.Logged("DatabaseFactory.Close", func() error {
		if f.db == nil {
			return nil
		}
		err := f.db.Close()
		f.db = nil
		if err != nil {
			f.log.Error("DatabaseFactory.Close: failed", "err", err)
			return errors.Wrap(errors.CodeDatabase, "close failed", err)
		}
		return nil
	})
}

// Migrate applies a list of SQL migrations in order, each in its own
// transaction, tracking applied versions in a schema_migrations table.
// Returns GEN-002 on failure. Since 1.4.0.
func (f *Factory) Migrate(ctx context.Context, migrations []Migration) error {
	return f.log.Logged("DatabaseFactory.Migrate", func() error {
		if f.db == nil {
			f.log.Error("DatabaseFactory.Migrate: Init not called")
			return errors.New(errors.CodeDatabase, "Init not called")
		}
		if err := f.ensureMigrationsTable(ctx); err != nil {
			return err
		}
		applied, err := f.appliedVersions(ctx)
		if err != nil {
			return err
		}
		for _, m := range migrations {
			if _, ok := applied[m.Version]; ok {
				continue
			}
			if err := f.applyOne(ctx, m); err != nil {
				return err
			}
		}
		return nil
	})
}

// Migration is a numbered SQL script. Since 1.4.0.
type Migration struct {
	Version string
	SQL     string
}

// Tx runs fn inside a transaction. On nil error it commits; on non-nil it
// rolls back and returns the error.
//
// v1.4.0 / P0-3.1: if fn returns an AppError, Tx returns it UNCHANGED. This
// preserves the caller's typed-error mapping (e.g. duplicate-key → API-003).
// Other errors are wrapped as GEN-002 after being logged at ERROR.
//
// Since 1.4.0.
func (f *Factory) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	return f.log.Logged("DatabaseFactory.Tx", func() error {
		if f.db == nil {
			f.log.Error("DatabaseFactory.Tx: Init not called")
			return errors.New(errors.CodeDatabase, "Init not called")
		}
		tx, err := f.db.BeginTx(ctx, nil)
		if err != nil {
			f.log.Error("DatabaseFactory.Tx: begin failed", "err", err)
			return errors.Wrap(errors.CodeDatabase, "begin tx failed", err)
		}
		committed := false
		defer func() {
			if !committed {
				if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
					f.log.Error("DatabaseFactory.Tx: rollback failed", "err", rbErr)
				}
			}
		}()
		if err := fn(tx); err != nil {
			// P0-3.1: AppError pass-through — do not wrap.
			if isAppError(err) {
				f.log.Error("DatabaseFactory.Tx: body returned AppError (pass-through)", "err", err)
				return err
			}
			f.log.Error("DatabaseFactory.Tx: body failed", "err", err)
			return errors.Wrap(errors.CodeDatabase, "tx body failed", err)
		}
		if err := tx.Commit(); err != nil {
			f.log.Error("DatabaseFactory.Tx: commit failed", "err", err)
			return errors.Wrap(errors.CodeDatabase, "commit failed", err)
		}
		committed = true
		return nil
	})
}

// Query runs a read query outside a transaction (introspection uses this).
// Since 1.4.0.
func (f *Factory) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return applogger.Logged1(f.log, "DatabaseFactory.Query", func() (*sql.Rows, error) {
		if f.db == nil {
			return nil, errors.New(errors.CodeDatabase, "Init not called")
		}
		rows, err := f.db.QueryContext(ctx, query, args...)
		if err != nil {
			f.log.Error("DatabaseFactory.Query: failed", "query", query, "err", err)
			return nil, errors.Wrap(errors.CodeDatabase, "query failed", err)
		}
		return rows, nil
	})
}

// ---- helpers ---------------------------------------------------------------

// ensureMigrationsTable creates the tracking table if absent. Since 1.4.0.
func (f *Factory) ensureMigrationsTable(ctx context.Context) error {
	_, err := f.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	if err != nil {
		f.log.Error("ensureMigrationsTable: failed", "err", err)
		return errors.Wrap(errors.CodeDatabase, "create schema_migrations failed", err)
	}
	return nil
}

// appliedVersions returns the set of already-applied migration versions.
// Since 1.4.0.
func (f *Factory) appliedVersions(ctx context.Context) (map[string]struct{}, error) {
	rows, err := f.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		f.log.Error("appliedVersions: query failed", "err", err)
		return nil, errors.Wrap(errors.CodeDatabase, "select schema_migrations failed", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			f.log.Error("appliedVersions: scan failed", "err", err)
			return nil, errors.Wrap(errors.CodeDatabase, "scan migration version failed", err)
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}

// applyOne runs a single migration inside a transaction and records it.
// Since 1.4.0.
func (f *Factory) applyOne(ctx context.Context, m Migration) error {
	return f.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, m.Version); err != nil {
			return err
		}
		return nil
	})
}

// isAppError reports whether err implements the generator's AppError contract
// (Code() string). Since 1.4.0.
func isAppError(err error) bool {
	type coder interface{ Code() string }
	_, ok := err.(coder)
	return ok
}

// String formats a Factory for debug logs. Since 1.4.0.
func (f *Factory) String() string {
	if f.db == nil {
		return "DatabaseFactory(uninitialised)"
	}
	s := f.db.Stats()
	return fmt.Sprintf("DatabaseFactory(open=%d in-use=%d)", s.OpenConnections, s.InUse)
}
