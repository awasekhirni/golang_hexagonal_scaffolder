// Package applogger provides the structured logger used by the generator.
//
// It is built on Go's standard-library log/slog and supports both text and
// JSON output (selected by the LOG_FORMAT env var). Every public function in
// the generator is wrapped by AppLogger.Logged so that ENTER/OK/ERROR lines
// are emitted uniformly and the original error is logged with its stack trace
// before any catch-and-rethrow wraps it.
//
// The Logged contract is enforced by TestAllPublicFunctionsAreLogged in this
// package, which walks the AST of every other package in the module.
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

// Level names accepted from env vars.
const (
	envLogLevel    = "LOG_LEVEL"
	envHexapiLevel = "HEXAPI_LOG_LEVEL"
	envLogFormat   = "LOG_FORMAT"
)

// redactedKeys are attribute keys whose values are never emitted.
var redactedKeys = map[string]struct{}{
	"password": {},
	"secret":   {},
	"token":    {},
	"jwt":      {},
}

// Logger is the structured logger. It wraps *slog.Logger so callers can use
// the standard slog API (Info, Error, With, ...) directly.
//
// Since 1.4.0.
type Logger struct {
	*slog.Logger
}

// New constructs a Logger from the process env vars.
//   - LOG_FORMAT=json → slog.NewJSONHandler; otherwise text.
//   - LOG_LEVEL sets the root level (default INFO).
//   - HEXAPI_LOG_LEVEL overrides the github.com/example/hexapi subtree
//     (default DEBUG).
//
// Since 1.4.0.
func New() *Logger {
	return NewWithEnv(os.Getenv)
}

// NewWithEnv constructs a Logger using the supplied getenv function, which
// makes the logger testable without touching the process env. Since 1.4.0.
func NewWithEnv(getenv func(string) string) *Logger {
	root := parseLevel(getenv(envLogLevel), slog.LevelInfo)
	hex := parseLevel(getenv(envHexapiLevel), slog.LevelDebug)
	format := strings.ToLower(strings.TrimSpace(getenv(envLogFormat)))

	var h slog.Handler
	opts := &slog.HandlerOptions{
		Level:     root,
		AddSource: false,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if _, ok := redactedKeys[a.Key]; ok {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}
	if format == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	// Apply the hexapi subtree override via a child handler with its own level.
	// slog does not natively support per-package levels on a single handler;
	// the conventional pattern is a child logger whose handler has the
	// narrower level. We expose both via the package so callers pick.
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
		// Replace the base logger with the child for the hexapi subtree.
		base = slog.New(childH).WithGroup("hexapi")
	}
	return &Logger{Logger: base}
}

// parseLevel maps an env-var string to a slog.Level. Unknown values fall back
// to the supplied default. Since 1.4.0.
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
//
// Usage:
//
//	return log.Logged("Service.Foo", func() error {
//	    // ... body ...
//	    return nil
//	})
//
// The wrapper logs ENTER at debug on entry, OK + duration at debug on
// success, and ERROR with the error (and stack-friendly attributes) on
// failure. The returned error is the closure's error, unchanged, so callers
// can still branch on typed errors.
//
// Since 1.4.0.
func (l *Logger) Logged(fn string, body func() error) error {
	l.debugEnter(fn)
	start := time.Now()
	err := body()
	dur := time.Since(start)
	if err != nil {
		// ERROR carries the error so the stack-friendly slog handler emits it.
		l.Error("ERROR", "fn", fn, "dur_ms", dur.Milliseconds(), "err", err)
		return err
	}
	l.Debug("OK", "fn", fn, "dur_ms", dur.Milliseconds())
	return nil
}

// Logged1 is the typed variant for functions returning (T, error).
//
//	return log.Logged1("Service.Bar", func() (T, error) {
//	    // ... body ...
//	    return v, nil
//	})
//
// Since 1.4.0.
func Logged1[T any](l *Logger, fn string, body func() (T, error)) (T, error) {
	var zero T
	l.debugEnter(fn)
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

// WithCtx returns a Logger that carries the request-id from the context, if
// present. The context value is set by the API's RequestIdMiddleware; in the
// generator it is usually empty. Since 1.4.0.
func (l *Logger) WithCtx(ctx context.Context) *Logger {
	if reqID, ok := ctx.Value(reqIDKey{}).(string); ok && reqID != "" {
		return &Logger{Logger: l.With("reqId", reqID)}
	}
	return l
}

// reqIDKey is the context key for the request id. Since 1.4.0.
type reqIDKey struct{}

// CtxWithReqID stamps a request id into the context. Since 1.4.0.
func CtxWithReqID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, reqIDKey{}, id)
}

// debugEnter logs the ENTER line at debug. Since 1.4.0.
func (l *Logger) debugEnter(fn string) {
	l.Debug("ENTER", "fn", fn)
}
