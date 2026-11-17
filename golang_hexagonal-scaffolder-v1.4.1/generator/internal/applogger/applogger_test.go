package applogger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// TestNew_textFormat covers the text handler default. Since 1.4.0.
func TestNew_textFormat(t *testing.T) {
	l := NewWithEnv(func(string) string { return "" })
	if l == nil || l.Logger == nil {
		t.Fatalf("New returned nil logger")
	}
}

// TestNew_jsonFormat covers the JSON handler. Since 1.4.0.
func TestNew_jsonFormat(t *testing.T) {
	// Capture stdout by swapping os.Stdout — too invasive for a unit test.
	// Instead, assert that LOG_FORMAT=json does not panic and produces a
	// non-nil logger.
	l := NewWithEnv(func(k string) string {
		if k == "LOG_FORMAT" {
			return "json"
		}
		return ""
	})
	if l == nil {
		t.Fatalf("New json returned nil")
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

// TestCtxReqID_roundTrip covers the request-id context helpers. Since 1.4.0.
func TestCtxReqID_roundTrip(t *testing.T) {
	ctx := CtxWithReqID(context.Background(), "abc")
	if got, _ := ctx.Value(reqIDKey{}).(string); got != "abc" {
		t.Fatalf("reqId: want abc, got %q", got)
	}
}

// TestParseLevel covers the env mapping. Since 1.4.0.
func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"DEBUG":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"WARN":    slog.LevelWarn,
		"WARNING": slog.LevelWarn,
		"ERROR":   slog.LevelError,
		"":        slog.LevelInfo, // default
		"garbage": slog.LevelInfo, // default
	}
	for in, want := range cases {
		if got := parseLevel(in, slog.LevelInfo); got != want {
			t.Fatalf("parseLevel(%q): want %v, got %v", in, want, got)
		}
	}
}

// TestRedaction covers that password/secret/token keys are redacted.
// Since 1.4.0.
func TestRedaction(t *testing.T) {
	// Build a logger writing to a buffer so we can inspect output.
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if _, ok := redactedKeys[a.Key]; ok {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}
	l := slog.New(slog.NewTextHandler(&buf, opts))
	l.Info("x", "password", "supersecret", "token", "abc", "user", "alice")
	out := buf.String()
	if strings.Contains(out, "supersecret") || strings.Contains(out, "token=abc") {
		t.Fatalf("redaction failed: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in output: %s", out)
	}
	if !strings.Contains(out, "user=alice") {
		t.Fatalf("non-redacted attr should pass through: %s", out)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
