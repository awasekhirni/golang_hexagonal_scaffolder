package settings

import (
	"strings"
	"testing"

	"github.com/example/generated-api/internal/shared/applogger"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }
type env map[string]string
func (e env) get(k string) string { return e[k] }

// TestToSettings_happy covers defaults. Since 1.4.0.
func TestToSettings_happy(t *testing.T) {
	s, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw"}.get)
	if err != nil {
		t.Fatalf("ToSettings: %v", err)
	}
	if s.DBPassword != "pw" {
		t.Fatalf("password: want pw, got %q", s.DBPassword)
	}
	if s.DBPort != 5432 {
		t.Fatalf("port default: want 5432, got %d", s.DBPort)
	}
	if s.PoolSize != 10 {
		t.Fatalf("pool default: want 10, got %d", s.PoolSize)
	}
}

// TestToSettings_missingPassword covers API-001. Since 1.4.0.
func TestToSettings_missingPassword(t *testing.T) {
	_, err := ToSettings(log(), env{}.get)
	if err == nil || !strings.Contains(err.Error(), "API-001") {
		t.Fatalf("missing password: want API-001, got %v", err)
	}
}

// TestToSettings_badPort covers the parse path. Since 1.4.0.
func TestToSettings_badPort(t *testing.T) {
	_, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "POSTGRES_PORT": "x"}.get)
	if err == nil || !strings.Contains(err.Error(), "API-001") {
		t.Fatalf("bad port: want API-001, got %v", err)
	}
}

// TestToSettings_badLogFormat covers the enum. Since 1.4.0.
func TestToSettings_badLogFormat(t *testing.T) {
	_, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "LOG_FORMAT": "xml"}.get)
	if err == nil || !strings.Contains(err.Error(), "API-001") {
		t.Fatalf("bad format: want API-001, got %v", err)
	}
}

// TestToSettings_CORSOrigins covers CSV parsing. Since 1.4.0.
func TestToSettings_CORSOrigins(t *testing.T) {
	s, _ := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "CORS_ALLOWED_ORIGINS": "http://a.com, http://b.com"}.get)
	if len(s.CORSOrigins) != 2 || s.CORSOrigins[0] != "http://a.com" {
		t.Fatalf("CORS: got %v", s.CORSOrigins)
	}
}

// TestToSettings_DSN covers the connection string. Since 1.4.0.
func TestToSettings_DSN(t *testing.T) {
	s, _ := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw"}.get)
	dsn := s.DSN()
	if !strings.Contains(dsn, "password=pw") || !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("DSN: %q", dsn)
	}
}
