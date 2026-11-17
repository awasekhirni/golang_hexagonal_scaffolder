// Package applogger provides the structured logger for the generated API.
//
// It mirrors the generator's applogger: built on log/slog, supports text and
// JSON output (LOG_FORMAT), env-configurable levels (LOG_LEVEL root,
// HEXAPI_LOG_LEVEL subtree), and the Logged wrapper for entry/exit/error
// tracing on every public function.
//
// Since 1.4.0.
package applogger

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	envLogLevel    = "LOG_LEVEL"
	envHexapiLevel = "HEXAPI_LOG_LEVEL"
	envLogFormat   = "LOG_FORMAT"
)

var redactedKeys = map[string]struct{}{
	"password": {},
	"secret":   {},
	"token":    {},
	"jwt":      {},
}

// Logger wraps *slog.Logger. Since 1.4.0.
type Logger struct {
	*slog.Logger
}

// New constructs a Logger from process env vars. Since 1.4.0.
func New() *Logger { return NewWithEnv(os.Getenv) }

// NewWithEnv constructs a Logger using the supplied getenv (testable).
// Since 1.4.0.
func NewWithEnv(getenv func(string) string) *Logger {
	root := parseLevel(getenv(envLogLevel), slog.LevelInfo)
	hex := parseLevel(getenv(envHexapiLevel), slog.LevelDebug)
	format := strings.ToLower(strings.TrimSpace(getenv(envLogFormat)))

	opts := &slog.HandlerOptions{
		Level: root,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if _, ok := redactedKeys[a.Key]; ok {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	base := slog.New(h)
	if hex != root {
		childOpts := *opts
		childOpts.Level = hex
		var childH slog.Handler
		if format == "json" {
			childH = slog.NewJSONHandler(os.Stdout, &childOpts)
		} else {
			childH = slog.NewTextHandler(os.Stdout, &childOpts)
		}
		base = slog.New(childH).WithGroup("hexapi")
	}
	return &Logger{Logger: base}
}

func parseLevel(s string, def slog.Level) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	case "INFO":
		return slog.LevelInfo
	case "":
		return def
	default:
		return def
	}
}

// Logged wraps a no-return closure with ENTER/OK/ERROR tracing.
// Since 1.4.0.
func (l *Logger) Logged(fn string, body func() error) error {
	l.Debug("ENTER", "fn", fn)
	start := time.Now()
	err := body()
	dur := time.Since(start)
	if err != nil {
		l.Error("ERROR", "fn", fn, "dur_ms", dur.Milliseconds(), "err", err)
		return err
	}
	l.Debug("OK", "fn", fn, "dur_ms", dur.Milliseconds())
	return nil
}

// Logged1 is the typed variant for (T, error). Since 1.4.0.
func Logged1[T any](l *Logger, fn string, body func() (T, error)) (T, error) {
	var zero T
	l.Debug("ENTER", "fn", fn)
	start := time.Now()
	v, err := body()
	dur := time.Since(start)
	if err != nil {
		l.Error("ERROR", "fn", fn, "dur_ms", dur.Milliseconds(), "err", err)
		return zero, err
	}
	l.Debug("OK", "fn", fn, "dur_ms", dur.Milliseconds())
	return v, nil
}

// reqIDKey is the context key for the request id. Since 1.4.0.
type reqIDKey struct{}

// CtxWithReqID stamps a request id into the context. Since 1.4.0.
func CtxWithReqID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, reqIDKey{}, id)
}

// ReqIDFromCtx extracts the request id, if present. Since 1.4.0.
func ReqIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(reqIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithCtx returns a Logger carrying the request id from context. Since 1.4.0.
func (l *Logger) WithCtx(ctx context.Context) *Logger {
	if id := ReqIDFromCtx(ctx); id != "" {
		return &Logger{Logger: l.With("reqId", id)}
	}
	return l
}
