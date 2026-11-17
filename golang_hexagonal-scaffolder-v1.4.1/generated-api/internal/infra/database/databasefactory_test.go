package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/example/generated-api/internal/shared/applogger"
	"github.com/example/generated-api/internal/testutil/fakedb"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// TestInit_happy covers opening against fakedb. Since 1.4.0.
func TestInit_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if f.DB() == nil {
		t.Fatalf("DB nil after Init")
	}
	_ = f.Close()
}

// TestInit_badDSN covers the unavailable path. Since 1.4.0.
func TestInit_badDSN(t *testing.T) {
	f := New(log())
	err := f.Init(context.Background(), "fakedb", "nonexistent", 5, 5, 5)
	if err == nil || !strings.Contains(err.Error(), "API-") {
		t.Fatalf("Init bad: want API- code, got %v", err)
	}
}

// TestTx_passThroughAppError is the P0-3.1 regression. Since 1.4.0.
func TestTx_passThroughAppError(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	appErr := typedAppErr{code: "API-003", msg: "duplicate"}
	err := f.Tx(context.Background(), func(*sql.Tx) error { return appErr })
	if err != appErr {
		t.Fatalf("Tx should pass AppError through; got %v want %v", err, appErr)
	}
}

// TestTx_wrapsGenericError covers the non-AppError path. Since 1.4.0.
func TestTx_wrapsGenericError(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	err := f.Tx(context.Background(), func(*sql.Tx) error { return errors.New("boom") })
	if err == nil || !strings.Contains(err.Error(), "API-007") {
		t.Fatalf("Tx generic: want API-007, got %v", err)
	}
}

// TestTx_commit_happy covers the success path. Since 1.4.0.
func TestTx_commit_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	called := false
	err := f.Tx(context.Background(), func(tx *sql.Tx) error { called = true; return nil })
	if err != nil || !called {
		t.Fatalf("Tx commit: err=%v called=%v", err, called)
	}
}

// TestPing_happy covers the readiness probe. Since 1.4.0.
func TestPing_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("SELECT 1", &fakedb.Rows{
		Cols:   []string{"one"},
		Values: [][]any{{1}},
	})
	f := New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	if err := f.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// typedAppErr is a minimal AppError for the pass-through test. Since 1.4.0.
type typedAppErr struct{ code, msg string }

func (e typedAppErr) Error() string     { return e.code + ": " + e.msg }
func (e typedAppErr) Code() string      { return e.code }
func (e typedAppErr) HTTPStatus() int   { return 409 }
func (e typedAppErr) Category() string  { return "conflict" }
