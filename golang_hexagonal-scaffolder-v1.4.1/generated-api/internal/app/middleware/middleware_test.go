package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/generated-api/internal/shared/applogger"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// TestRequestID_setsHeader covers the X-Request-ID response header. Since 1.4.0.
func TestRequestID_setsHeader(t *testing.T) {
	h := RequestID(log())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("X-Request-ID header missing")
	}
}

// TestRequestID_preservesIncoming covers an incoming X-Request-ID. Since 1.4.0.
func TestRequestID_preservesIncoming(t *testing.T) {
	h := RequestID(log())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "incoming-id")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "incoming-id" {
		t.Fatalf("X-Request-ID: want incoming-id, got %q", got)
	}
}

// TestRecover_convertsPanic covers API-007 on panic. Since 1.4.0.
func TestRecover_convertsPanic(t *testing.T) {
	h := Recover(log())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatalf("status: want 500, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-007" {
		t.Fatalf("code: want API-007, got %q", body["code"])
	}
}

// TestSecurityHeaders_present covers all headers. Since 1.4.0.
func TestSecurityHeaders_present(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	checks := map[string]string{
		"Content-Security-Policy":   "default-src 'self'",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Referrer-Policy":           "no-referrer",
		"Cache-Control":             "no-store",
	}
	for h, v := range checks {
		if got := rec.Header().Get(h); got != v {
			t.Fatalf("%s: want %q, got %q", h, v, got)
		}
	}
}

// TestCORS_allowedOrigin covers P0-7.9: allowlist, never *. Since 1.4.0.
func TestCORS_allowedOrigin(t *testing.T) {
	h := CORS([]string{"http://good.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://good.com")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://good.com" {
		t.Fatalf("CORS allowed: want http://good.com, got %q", got)
	}
}

// TestCORS_blockedOrigin covers the deny path. Since 1.4.0.
func TestCORS_blockedOrigin(t *testing.T) {
	h := CORS([]string{"http://good.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS blocked: want empty, got %q", got)
	}
}

// TestCORS_preflight covers OPTIONS. Since 1.4.0.
func TestCORS_preflight(t *testing.T) {
	h := CORS([]string{"http://good.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called on preflight")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "http://good.com")
	h.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("preflight: want 204, got %d", rec.Code)
	}
}

// TestRateLimit_allowsUnderBudget covers the happy path. Since 1.4.0.
func TestRateLimit_allowsUnderBudget(t *testing.T) {
	rl := NewRateLimiter(100)
	called := 0
	h := rl.RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(200)
	}))
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("iter %d: want 200, got %d", i, rec.Code)
		}
	}
	if called != 10 {
		t.Fatalf("called: want 10, got %d", called)
	}
}

// TestRateLimit_blocksOverBudget covers API-005. Since 1.4.0.
func TestRateLimit_blocksOverBudget(t *testing.T) {
	rl := NewRateLimiter(2)
	h := rl.RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	// First two succeed.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("iter %d: want 200, got %d", i, rec.Code)
		}
	}
	// Third is blocked.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 429 {
		t.Fatalf("over budget: want 429, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-005" {
		t.Fatalf("code: want API-005, got %q", body["code"])
	}
}

// TestAuth_missingHeader covers API-004. Since 1.4.0.
func TestAuth_missingHeader(t *testing.T) {
	h := Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-004" {
		t.Fatalf("code: want API-004, got %q", body["code"])
	}
}

// TestAuth_validToken covers the happy path. Since 1.4.0.
func TestAuth_validToken(t *testing.T) {
	secret := "test-secret"
	token := IssueToken(secret, `{"sub":"alice"}`)
	h := Auth(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
}

// TestAuth_invalidToken covers API-004. Since 1.4.0.
func TestAuth_invalidToken(t *testing.T) {
	h := Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.token")
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
}

// TestWriteError_appError covers the typed-error renderer. Since 1.4.0.
func TestWriteError_appError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(applogger.CtxWithReqID(req.Context(), "req-1"))
	WriteError(rec, req, hexerrNotFound("gone"), log())
	if rec.Code != 404 {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["code"] != "API-002" || body["reqId"] != "req-1" {
		t.Fatalf("body: %+v", body)
	}
}

// hexerrNotFound is a minimal AppError for the test. Since 1.4.0.
type hexerrNotFound string

func (e hexerrNotFound) Error() string     { return string(e) }
func (e hexerrNotFound) Code() string      { return "API-002" }
func (e hexerrNotFound) HTTPStatus() int   { return 404 }
func (e hexerrNotFound) Category() string  { return "not-found" }

// TestClientIP covers the extraction. Since 1.4.0.
func TestClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
	if got := clientIP(req); !strings.HasPrefix(got, "9.9.9.9") {
		t.Fatalf("clientIP: want 9.9.9.9, got %q", got)
	}
}

// TestConstantTimeEq covers the comparison. Since 1.4.0.
func TestConstantTimeEq(t *testing.T) {
	if !constantTimeEq("abc", "abc") {
		t.Fatalf("eq: want true")
	}
	if constantTimeEq("abc", "abd") {
		t.Fatalf("eq: want false")
	}
	if constantTimeEq("abc", "ab") {
		t.Fatalf("eq diff len: want false")
	}
}

// silence unused
var _ = context.Background
var _ = time.Now
