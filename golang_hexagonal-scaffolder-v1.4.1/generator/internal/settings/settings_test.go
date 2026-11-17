package settings

import (
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

// env helper for tests.
type env map[string]string

func (e env) get(k string) string { return e[k] }

// TestToSettings_happy covers defaults + env overlay. Since 1.4.0.
func TestToSettings_happy(t *testing.T) {
	s, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw"}.get, Flags{})
	if err != nil {
		t.Fatalf("ToSettings: %v", err)
	}
	if s.DBPassword != "pw" {
		t.Fatalf("password: want pw, got %q", s.DBPassword)
	}
	if s.DBPort != 5432 {
		t.Fatalf("port default: want 5432, got %d", s.DBPort)
	}
	if s.OutputDir != "./out" {
		t.Fatalf("output default: want ./out, got %q", s.OutputDir)
	}
}

// TestToSettings_flagsOverride covers flag precedence. Since 1.4.0.
func TestToSettings_flagsOverride(t *testing.T) {
	s, err := ToSettings(log(), env{"POSTGRES_HOST": "from-env"}.get, Flags{DBHost: "from-flag", DBPort: 9999, DBPassword: "pw"})
	if err != nil {
		t.Fatalf("ToSettings: %v", err)
	}
	if s.DBHost != "from-flag" {
		t.Fatalf("host: want from-flag, got %q", s.DBHost)
	}
	if s.DBPort != 9999 {
		t.Fatalf("port: want 9999, got %d", s.DBPort)
	}
}

// TestToSettings_missingPassword covers the GEN-001 required path. Since 1.4.0.
func TestToSettings_missingPassword(t *testing.T) {
	_, err := ToSettings(log(), env{}.get, Flags{})
	if err == nil || !strings.Contains(err.Error(), "GEN-001") {
		t.Fatalf("missing password: want GEN-001, got %v", err)
	}
}

// TestToSettings_badPort covers the GEN-001 parse path with cause logging.
// Since 1.4.0.
func TestToSettings_badPort(t *testing.T) {
	_, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "POSTGRES_PORT": "not-an-int"}.get, Flags{})
	if err == nil || !strings.Contains(err.Error(), "GEN-001") {
		t.Fatalf("bad port: want GEN-001, got %v", err)
	}
}

// TestToSettings_portOutOfRange covers the range check. Since 1.4.0.
func TestToSettings_portOutOfRange(t *testing.T) {
	_, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "POSTGRES_PORT": "99999"}.get, Flags{})
	if err == nil || !strings.Contains(err.Error(), "GEN-001") {
		t.Fatalf("port out of range: want GEN-001, got %v", err)
	}
}

// TestToSettings_badLogFormat covers the LOG_FORMAT enum. Since 1.4.0.
func TestToSettings_badLogFormat(t *testing.T) {
	_, err := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw", "LOG_FORMAT": "xml"}.get, Flags{})
	if err == nil || !strings.Contains(err.Error(), "GEN-001") {
		t.Fatalf("bad log format: want GEN-001, got %v", err)
	}
}

// TestToSettings_DSN covers the connection-string builder. Since 1.4.0.
func TestToSettings_DSN(t *testing.T) {
	s, _ := ToSettings(log(), env{"POSTGRES_PASSWORD": "pw"}.get, Flags{})
	dsn := s.DSN()
	if !strings.Contains(dsn, "password=pw") || !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("DSN: want password=pw and sslmode=disable, got %q", dsn)
	}
}
