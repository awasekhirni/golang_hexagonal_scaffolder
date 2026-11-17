package databasefactory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/testutil/fakedb"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// TestInit_happy covers opening against the fakedb driver. Since 1.4.0.
func TestInit_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if f.DB() == nil {
		t.Fatalf("DB should be non-nil after Init")
	}
	_ = f.Close()
}

// TestInit_badDSN covers the GEN-002 path (unknown fakedb name). Since 1.4.0.
func TestInit_badDSN(t *testing.T) {
	f := New(log())
	err := f.Init(context.Background(), "fakedb", "nonexistent-fake-db-name")
	if err == nil || !strings.Contains(err.Error(), "GEN-002") {
		t.Fatalf("Init bad: want GEN-002, got %v", err)
	}
}

// TestTx_passThroughAppError is the P0-3.1 regression: if the body returns
// an AppError (something implementing Code()), Tx must return it UNCHANGED,
// not wrap it as GEN-002. Since 1.4.0.
func TestTx_passThroughAppError(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	appErr := typedAppErr{code: "API-003", msg: "duplicate"}
	err := f.Tx(context.Background(), func(*sql.Tx) error { return appErr })
	if err != appErr {
		t.Fatalf("Tx should pass AppError through unchanged; got %v want %v", err, appErr)
	}
}

// typedAppErr is a minimal AppError for the pass-through test. Since 1.4.0.
type typedAppErr struct{ code, msg string }

func (e typedAppErr) Error() string { return e.code + ": " + e.msg }
func (e typedAppErr) Code() string  { return e.code }

// TestTx_wrapsGenericError covers the non-AppError path. Since 1.4.0.
func TestTx_wrapsGenericError(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	err := f.Tx(context.Background(), func(*sql.Tx) error { return errors.New("boom") })
	if err == nil || !strings.Contains(err.Error(), "GEN-002") {
		t.Fatalf("Tx generic: want GEN-002, got %v", err)
	}
}

// TestTx_commit_happy covers the success path. Since 1.4.0.
func TestTx_commit_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	called := false
	err := f.Tx(context.Background(), func(tx *sql.Tx) error { called = true; return nil })
	if err != nil || !called {
		t.Fatalf("Tx commit: err=%v called=%v", err, called)
	}
}

// TestMigrate_idempotent covers running migrations twice. Since 1.4.0.
func TestMigrate_idempotent(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	// CREATE TABLE and SELECT both need canned results.
	fake.AddRows("schema_migrations", &fakedb.Rows{
		Cols: []string{"version"},
	})
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	migs := []Migration{{Version: "0001", SQL: "SELECT 1"}}
	if err := f.Migrate(context.Background(), migs); err != nil {
		t.Fatalf("Migrate first: %v", err)
	}
	if err := f.Migrate(context.Background(), migs); err != nil {
		t.Fatalf("Migrate second (idempotent): %v", err)
	}
}
