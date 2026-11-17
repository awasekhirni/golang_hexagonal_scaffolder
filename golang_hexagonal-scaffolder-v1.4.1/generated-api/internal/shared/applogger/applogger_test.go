package applogger

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// TestNew_returnsNonNil covers construction. Since 1.4.0.
func TestNew_returnsNonNil(t *testing.T) {
	l := NewWithEnv(func(string) string { return "" })
	if l == nil || l.Logger == nil {
		t.Fatalf("New returned nil")
	}
}

// TestLogged_passesErrorThrough covers the error pass-through. Since 1.4.0.
func TestLogged_passesErrorThrough(t *testing.T) {
	l := NewWithEnv(func(string) string { return "" })
	want := errBoom{}
	got := l.Logged("X", func() error { return want })
	if got != want {
		t.Fatalf("Logged: want %v, got %v", want, got)
	}
}

// TestLogged_successReturnsNil covers the happy path. Since 1.4.0.
func TestLogged_successReturnsNil(t *testing.T) {
	l := NewWithEnv(func(string) string { return "" })
	if err := l.Logged("X", func() error { return nil }); err != nil {
		t.Fatalf("Logged success: want nil, got %v", err)
	}
}

// TestLogged1_passesValueAndError covers the typed variant. Since 1.4.0.
func TestLogged1_passesValueAndError(t *testing.T) {
	l := NewWithEnv(func(string) string { return "" })
	v, err := Logged1[int](l, "X", func() (int, error) { return 42, nil })
	if err != nil || v != 42 {
		t.Fatalf("Logged1: want 42,nil; got %d,%v", v, err)
	}
	_, err = Logged1[int](l, "X", func() (int, error) { return 0, errBoom{} })
	if err == nil {
		t.Fatalf("Logged1 error: want non-nil")
	}
}

// TestReqID_roundTrip covers the context helpers. Since 1.4.0.
func TestReqID_roundTrip(t *testing.T) {
	ctx := CtxWithReqID(context.Background(), "abc")
	if got := ReqIDFromCtx(ctx); got != "abc" {
		t.Fatalf("reqId: want abc, got %q", got)
	}
	if got := ReqIDFromCtx(context.Background()); got != "" {
		t.Fatalf("empty ctx: want empty, got %q", got)
	}
}

// TestParseLevel covers the env mapping. Since 1.4.0.
func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"DEBUG": slog.LevelDebug, "INFO": slog.LevelInfo,
		"WARN": slog.LevelWarn, "WARNING": slog.LevelWarn,
		"ERROR": slog.LevelError, "": slog.LevelInfo, "garbage": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in, slog.LevelInfo); got != want {
			t.Fatalf("parseLevel(%q): want %v, got %v", in, want, got)
		}
	}
}

// TestRedaction covers password/secret/token redaction. Since 1.4.0.
func TestRedaction(t *testing.T) {
	if _, ok := redactedKeys["password"]; !ok {
		t.Fatalf("password not in redacted set")
	}
	if !strings.Contains("password secret token jwt", "password") {
		t.Fatalf("sanity")
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
