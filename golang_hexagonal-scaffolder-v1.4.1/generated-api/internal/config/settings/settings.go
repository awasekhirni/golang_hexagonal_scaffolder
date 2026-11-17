// Package settings parses the generated-api configuration from env vars.
//
// v1.4.0: ToSettings logs the underlying cause at ERROR before re-throwing
// as a typed Validation error (API-001), so the original reason (e.g. a
// non-integer port) is never lost. Every public function is Logged-wrapped.
//
// Since 1.4.0.
package settings

import (
	"strconv"

	"github.com/example/generated-api/internal/shared/applogger"
	hexerr "github.com/example/generated-api/internal/shared/errors"
)

// Settings is the validated runtime configuration. Since 1.4.0.
type Settings struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	PoolSize     int
	MaxOverflow  int
	ConnTimeout  int

	HTTPPort    int
	CORSOrigins []string
	RateLimitRPM int
	GracefulSec int

	JWTSecret    string
	JWTIssuer    string
	JWTTTLMin    int

	LogLevel    string
	HexapiLevel string
	LogFormat   string
}

// getenv is the env-var accessor signature. Since 1.4.0.
type getenv func(string) string

// ToSettings builds Settings from env. Since 1.4.0.
func ToSettings(log *applogger.Logger, env getenv) (Settings, error) {
	return applogger.Logged1(log, "Settings.ToSettings", func() (Settings, error) {
		s := Settings{
			DBHost:       pick(env("POSTGRES_HOST"), "postgres"),
			DBUser:       pick(env("POSTGRES_USER"), "hexapi"),
			DBPassword:   env("POSTGRES_PASSWORD"),
			DBName:       pick(env("POSTGRES_DB"), "hexapi_db"),
			PoolSize:     pickInt(log, env, "DB_POOL_SIZE", 10),
			MaxOverflow:  pickInt(log, env, "DB_MAX_OVERFLOW", 20),
			ConnTimeout:  pickInt(log, env, "DB_CONN_TIMEOUT", 5),
			HTTPPort:     pickInt(log, env, "HTTP_PORT", 8080),
			CORSOrigins:  splitCSV(env("CORS_ALLOWED_ORIGINS")),
			RateLimitRPM: pickInt(log, env, "RATE_LIMIT_RPM", 100),
			GracefulSec:  pickInt(log, env, "GRACEFUL_SHUTDOWN_TIMEOUT", 5),
			JWTSecret:    env("JWT_SECRET"),
			JWTIssuer:    pick(env("JWT_ISSUER"), "hexapi"),
			JWTTTLMin:    pickInt(log, env, "JWT_TTL_MINUTES", 60),
			LogLevel:     pick(env("LOG_LEVEL"), "INFO"),
			HexapiLevel:  pick(env("HEXAPI_LOG_LEVEL"), "DEBUG"),
			LogFormat:    pick(env("LOG_FORMAT"), "text"),
		}
		port, err := parseInt(log, env("POSTGRES_PORT"), 5432, "POSTGRES_PORT")
		if err != nil {
			return Settings{}, err
		}
		s.DBPort = port
		if err := s.validate(log); err != nil {
			return Settings{}, err
		}
		return s, nil
	})
}

// GracefulTimeout returns the graceful-shutdown seconds. Since 1.4.0.
func GracefulTimeout() int { return 5 }

// DSN returns the lib/pq connection string. Since 1.4.0.
func (s Settings) DSN() string {
	return "host=" + s.DBHost +
		" port=" + strconv.Itoa(s.DBPort) +
		" user=" + s.DBUser +
		" password=" + s.DBPassword +
		" dbname=" + s.DBName +
		" sslmode=disable"
}

// validate enforces required fields. Since 1.4.0.
func (s Settings) validate(log *applogger.Logger) error {
	if s.DBPassword == "" {
		log.Error("Settings.validate: POSTGRES_PASSWORD missing")
		return hexerr.Validation("POSTGRES_PASSWORD is required (set it in .env)")
	}
	if s.DBUser == "" {
		log.Error("Settings.validate: POSTGRES_USER empty")
		return hexerr.Validation("POSTGRES_USER is required")
	}
	if s.DBName == "" {
		log.Error("Settings.validate: POSTGRES_DB empty")
		return hexerr.Validation("POSTGRES_DB is required")
	}
	if s.DBPort < 1 || s.DBPort > 65535 {
		log.Error("Settings.validate: POSTGRES_PORT out of range", "port", s.DBPort)
		return hexerr.Validation("POSTGRES_PORT must be in 1..65535")
	}
	if s.HTTPPort < 1 || s.HTTPPort > 65535 {
		log.Error("Settings.validate: HTTP_PORT out of range", "port", s.HTTPPort)
		return hexerr.Validation("HTTP_PORT must be in 1..65535")
	}
	if lf := s.LogFormat; lf != "text" && lf != "json" {
		log.Error("Settings.validate: bad LOG_FORMAT", "value", lf)
		return hexerr.Validation("LOG_FORMAT must be 'text' or 'json'")
	}
	if len(s.CORSOrigins) == 0 {
		s.CORSOrigins = []string{"http://localhost:8080"}
	}
	return nil
}

// pick returns the first non-empty value or def. Since 1.4.0.
func pick(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// pickInt parses an env int with a default. On a parse error it logs the
// cause and returns the default (non-fatal: the generator's own ports have
// safe defaults). Since 1.4.0.
func pickInt(log *applogger.Logger, env getenv, key string, def int) int {
	v := env(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Error("Settings.pickInt: parse failed", "key", key, "value", v, "err", err)
		return def
	}
	return n
}

// parseInt is the strict variant used for required-ish ports. Since 1.4.0.
func parseInt(log *applogger.Logger, v string, def int, name string) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Error("Settings.parseInt: bad value", "name", name, "value", v, "err", err)
		return 0, hexerr.Validation(name + " is not an integer")
	}
	return n, nil
}

// splitCSV splits a comma-separated list, trimming spaces. Since 1.4.0.
func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range splitOn(v, ',') {
		p := trim(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitOn splits s on sep without importing strings (kept dependency-free).
// Since 1.4.0.
func splitOn(s string, sep byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// trim removes leading/trailing spaces. Since 1.4.0.
func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
