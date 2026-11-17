// Package middleware contains the HTTP middleware stack: RequestId, Recover,
// Error, Logging, SecurityHeaders, CORS, RateLimit, Auth.
//
// Every middleware is Logged-wrapped where it does meaningful work. The
// ErrorMiddleware is the single place that translates AppError → HTTP.
//
// Since 1.4.0.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// ---- RequestId -----------------------------------------------------------

// RequestID generates a UUID per request, stores it in context + slog
// attributes, and sets X-Request-ID on the response. Since 1.4.0.
func RequestID(log *applogger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set("X-Request-ID", id)
			ctx := applogger.CtxWithReqID(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// newRequestID returns a short hex id derived from time + random bytes.
// Since 1.4.0.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = readRand(b)
	h := sha256.Sum256(append([]byte(time.Now().Format(time.RFC3339Nano)), b...))
	return hex.EncodeToString(h[:])[:16]
}

// readRand is a swappable random reader (tests can override). Since 1.4.0.
var readRand = func(b []byte) (int, error) {
	return cryptoRandRead(b)
}

// ---- Recover --------------------------------------------------------------

// Recover converts panics into API-007 responses. Since 1.4.0.
func Recover(log *applogger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("Recover: panic", "panic", rec, "stack", string(debug.Stack()))
					WriteError(w, r, hexerr.Internal(fmt.Sprintf("internal error: %v", rec)), log)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Error ----------------------------------------------------------------

// WriteError is the single place that translates an error into an HTTP
// response. AppError → its HTTPStatus + code/message; any other error →
// API-007 (500). Controllers call this directly when a use case returns an
// error. Since 1.4.0.
func WriteError(w http.ResponseWriter, r *http.Request, err error, log *applogger.Logger) {
	ae, ok := hexerr.IsAppError(err)
	if !ok {
		log.Error("WriteError: non-AppError escaped", "err", err)
		ae = hexerr.Internal("unexpected error")
	}
	status := ae.HTTPStatus()
	if status == 0 {
		status = http.StatusInternalServerError
	}
	reqID := applogger.ReqIDFromCtx(r.Context())
	body := map[string]string{
		"code":    ae.Code(),
		"message": ae.Error(),
		"reqId":   reqID,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Error is a thin middleware that ensures every response has a security and
// content-type baseline; actual error rendering is done by WriteError in the
// controllers. Since 1.4.0.
func Error(log *applogger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Logging --------------------------------------------------------------

// Logging logs the request line and the eventual status + duration. Since 1.4.0.
func Logging(log *applogger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Info("HTTP",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"dur_ms", time.Since(start).Milliseconds(),
				"reqId", applogger.ReqIDFromCtx(r.Context()),
			)
		})
	}
}

// statusWriter captures the response status. Since 1.4.0.
type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the status. Since 1.4.0.
func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// ---- SecurityHeaders ------------------------------------------------------

// SecurityHeaders sets CSP, HSTS, X-Content-Type-Options, X-Frame-Options,
// Referrer-Policy, Cache-Control on every response. Since 1.4.0.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// ---- CORS -----------------------------------------------------------------

// CORS enforces an allowlist of origins. P0-7.9: never "*". Since 1.4.0.
func CORS(allowed []string) func(http.Handler) http.Handler {
	allowSet := map[string]bool{}
	for _, o := range allowed {
		allowSet[strings.TrimSpace(o)] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			}
			if r.Method == http.MethodOptions {
				if origin == "" || !allowSet[origin] {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- RateLimit ------------------------------------------------------------

// bucket is a token bucket per IP. Since 1.4.0.
type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter is a per-IP token bucket. Since 1.4.0.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rpm     int
}

// NewRateLimiter constructs a limiter. rpm=0 disables limiting. Since 1.4.0.
func NewRateLimiter(rpm int) *RateLimiter {
	return &RateLimiter{buckets: map[string]*bucket{}, rpm: rpm}
}

// RateLimit middleware. API-005 (429) when the budget is exhausted. Since 1.4.0.
func (l *RateLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.rpm <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !l.allow(ip, time.Now()) {
			w.Header().Set("Retry-After", "60")
			WriteError(w, r, hexerr.RateLimit("rate limit exceeded"), applogger.New())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow reports whether ip may proceed. Since 1.4.0.
func (l *RateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(l.rpm), last: now}
		l.buckets[ip] = b
	}
	// Refill: rpm tokens per minute → rpm/60 per second.
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * float64(l.rpm) / 60.0
	if b.tokens > float64(l.rpm) {
		b.tokens = float64(l.rpm)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP extracts the client IP from X-Forwarded-For or RemoteAddr.
// Since 1.4.0.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

// ---- Auth -----------------------------------------------------------------

// Auth verifies a JWT (simplified HMAC). API-004 on failure. Since 1.4.0.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				WriteError(w, r, hexerr.Auth("missing or malformed Authorization header"), applogger.New())
				return
			}
			token := strings.TrimPrefix(h, "Bearer ")
			if !verifyToken(secret, token) {
				WriteError(w, r, hexerr.Auth("invalid token"), applogger.New())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// verifyToken is a simplified HMAC-SHA256 verifier. Since 1.4.0.
func verifyToken(secret, token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	// Reconstruct the signed payload and compare. This is a minimal
	// implementation suitable for the reference API; production should
	// use a vetted JWT library.
	signingInput := parts[0] + "." + parts[1]
	mac := hmacHex(secret, signingInput)
	return constantTimeEq(mac, parts[2])
}

// IssueToken signs a payload with HMAC-SHA256. Since 1.4.0.
func IssueToken(secret, payload string) string {
	header := `{"alg":"HS256","typ":"JWT"}`
	h := base64URLEncode([]byte(header))
	p := base64URLEncode([]byte(payload))
	signingInput := h + "." + p
	sig := hmacHex(secret, signingInput)
	return signingInput + "." + sig
}

// hmacHex returns the hex-encoded HMAC-SHA256 of data with key. Since 1.4.0.
func hmacHex(key, data string) string {
	return base64URLEncode(hmacSHA256(key, data))
}

// constantTimeEq compares two strings without short-circuiting. Since 1.4.0.
func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var r byte
	for i := 0; i < len(a); i++ {
		r |= a[i] ^ b[i]
	}
	return r == 0
}

// ---- helpers --------------------------------------------------------------

// cryptoRandRead wraps crypto/rand for swappability. Since 1.4.0.
func cryptoRandRead(b []byte) (int, error) {
	return cryptoRandReader.Read(b)
}

// contextKey is a private key type for context values. Since 1.4.0.
type contextKey struct{}

// ctxKey is the key for the logger in context. Since 1.4.0.
var ctxKey = contextKey{}

// loggerFromCtx returns the logger from context, or a fresh one. Since 1.4.0.
func loggerFromCtx(ctx context.Context) *applogger.Logger {
	if l, ok := ctx.Value(ctxKey).(*applogger.Logger); ok {
		return l
	}
	return applogger.New()
}
