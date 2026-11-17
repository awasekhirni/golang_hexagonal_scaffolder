// server_integration_test.go: end-to-end test that builds the full chi router
// (middleware stack + routes) and exercises it via httptest, backed by the
// fakedb driver. Since 1.4.0.
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/generated-api/internal/config/settings"
	"github.com/example/generated-api/internal/domain/user/repository"
	"github.com/example/generated-api/internal/infra/bus"
	"github.com/example/generated-api/internal/infra/database"
	"github.com/example/generated-api/internal/shared/applogger"
	"github.com/example/generated-api/internal/testutil/fakedb"
)

// newTestDeps builds a Deps wired to a fakedb-backed factory. Since 1.4.0.
func newTestDeps(t *testing.T) (Deps, func()) {
	t.Helper()
	fake := fakedb.Register(t.Name())
	// SELECT 1 for readiness probes.
	fake.AddRows("SELECT 1", &fakedb.Rows{Cols: []string{"one"}, Values: [][]any{{1}}})
	// schema_migrations + users queries.
	fake.AddRows("schema_migrations", &fakedb.Rows{Cols: []string{"version"}})
	fake.AddRows("FROM users", &fakedb.Rows{
		Cols: []string{"id", "email", "name", "created_at", "updated_at"},
	})
	log := applogger.NewWithEnv(func(string) string { return "" })
	f := database.New(log)
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	repo := repository.NewUserPostgres(log, f)
	s := settings.Settings{
		HTTPPort:      8080,
		CORSOrigins:   []string{"http://localhost:8080"},
		RateLimitRPM:  100,
		GracefulSec:   5,
		JWTSecret:     "test-secret",
		JWTIssuer:     "test",
		JWTTTLMin:     60,
		LogLevel:      "INFO",
		HexapiLevel:   "DEBUG",
		LogFormat:     "text",
		DBHost:        "fake",
		DBPort:        1,
		DBUser:        "fake",
		DBPassword:    "fake",
		DBName:        "fake",
		PoolSize:      5,
		MaxOverflow:   5,
		ConnTimeout:   5,
	}
	d := Deps{
		Log:      log,
		Settings: s,
		DB:       f,
		UserRepo: repo,
		EventBus: bus.NewDomainEventBus(log),
		CmdBus:   bus.NewEnterpriseBus(log),
		Services: bus.NewServiceRegistration(log),
	}
	return d, func() { _ = f.Close(); fakedb.Reset() }
}

// TestServer_healthLive_returns200 exercises the full router for /health/live.
// Since 1.4.0.
func TestServer_healthLive_returns200(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health/live")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
}

// TestServer_healthReady_returns200 exercises the readiness probe through the
// full middleware stack. Since 1.4.0.
func TestServer_healthReady_returns200(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health/ready")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
}

// TestServer_usersWithoutAuth_returns401 verifies the auth middleware guards
// the /users route end-to-end. Since 1.4.0.
func TestServer_usersWithoutAuth_returns401(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/users")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status: want 401, got %d", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["code"] != "API-004" {
		t.Fatalf("code: want API-004, got %q", body["code"])
	}
}

// TestServer_usersWithAuth_returns200 verifies the full authenticated path
// through middleware → controller → service → repository. Since 1.4.0.
func TestServer_usersWithAuth_returns200(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	// Obtain a token via /auth/token.
	token := obtainToken(t, srv.URL)
	req, _ := http.NewRequest("GET", srv.URL+"/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 200, got %d: %s", resp.StatusCode, body)
	}
}

// TestServer_requestIDHeader_set verifies the RequestId middleware end-to-end.
// Since 1.4.0.
func TestServer_requestIDHeader_set(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/health/live", nil)
	req.Header.Set("X-Request-ID", "my-req-id")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "my-req-id" {
		t.Fatalf("X-Request-ID: want my-req-id, got %q", got)
	}
}

// TestServer_securityHeaders_set verifies SecurityHeaders end-to-end. Since 1.4.0.
func TestServer_securityHeaders_set(t *testing.T) {
	d, cleanup := newTestDeps(t)
	defer cleanup()
	srv := httptest.NewServer(Build(d))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health/live")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security header missing")
	}
}

// obtainToken gets a JWT from /auth/token for the test secret. Since 1.4.0.
func obtainToken(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Post(base+"/auth/token", "application/json",
		strings.NewReader(`{"email":"a@b.com","password":"x"}`))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("auth status: %d: %s", resp.StatusCode, body)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	tok, _ := body["token"].(string)
	if tok == "" {
		t.Fatalf("no token in response: %v", body)
	}
	return tok
}
