package routes

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/generated-api/internal/infra/database"
	"github.com/example/generated-api/internal/shared/applogger"
	"github.com/example/generated-api/internal/testutil/fakedb"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// TestLive_returns200 covers the liveness probe. Since 1.4.0.
func TestLive_returns200(t *testing.T) {
	h := NewHealthRoute(log(), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/live", nil)
	h.Live(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Live: want 200, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("Live body: %+v", body)
	}
}

// TestReady_nilDB_returns503 covers the unavailable path. Since 1.4.0.
func TestReady_nilDB_returns503(t *testing.T) {
	h := NewHealthRoute(log(), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/ready", nil)
	h.Ready(rec, req)
	if rec.Code != 503 {
		t.Fatalf("Ready nil DB: want 503, got %d", rec.Code)
	}
}

// TestReady_healthyDB_returns200 covers P0-7.1: probes DB with SELECT 1.
// Since 1.4.0.
func TestReady_healthyDB_returns200(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("SELECT 1", &fakedb.Rows{
		Cols:   []string{"one"},
		Values: [][]any{{1}},
	})
	f := database.New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	h := NewHealthRoute(log(), f)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/ready", nil)
	h.Ready(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Ready healthy: want 200, got %d", rec.Code)
	}
}

// TestCombined_healthyDB_returns200 covers the backwards-compatible probe.
// Since 1.4.0.
func TestCombined_healthyDB_returns200(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("SELECT 1", &fakedb.Rows{
		Cols:   []string{"one"},
		Values: [][]any{{1}},
	})
	f := database.New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	h := NewHealthRoute(log(), f)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	h.Combined(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Combined: want 200, got %d", rec.Code)
	}
}

// TestCombined_unhealthyDB_returns503 covers P0-7.1: /health no longer
// returns ok when the DB is down. Since 1.4.0.
func TestCombined_unhealthyDB_returns503(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	// Register no canned result → the SELECT 1 query fails to find a match.
	f := database.New(log())
	if err := f.Init(context.Background(), "fakedb", fake.Name(), 5, 5, 5); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer f.Close()
	h := NewHealthRoute(log(), f)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	h.Combined(rec, req)
	if rec.Code != 503 {
		t.Fatalf("Combined unhealthy: want 503, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "degraded" {
		t.Fatalf("Combined body: %+v", body)
	}
}

// TestMetrics_returns200 covers the metrics endpoint. Since 1.4.0.
func TestMetrics_returns200(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	Metrics(rec, req)
	if rec.Code != 200 {
		t.Fatalf("Metrics: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "#") {
		t.Fatalf("Metrics body should contain prometheus text")
	}
}
