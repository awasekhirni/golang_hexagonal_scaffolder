// Package settings parses the generator's configuration from env vars and
// CLI flags into a typed Settings struct.
//
// Every required-but-missing or invalid value produces a typed ConfigError
// (GEN-001). As of v1.4.0, ToSettings logs the underlying cause at ERROR
// before re-throwing the typed error, so the original reason (e.g. a
// non-integer port) is never lost.
//
// Since 1.4.0.
package settings

import (
	"strconv"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
)

// Settings is the fully-validated configuration for a generator run.
// Since 1.4.0.
type Settings struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	OutputDir  string
	ModulePath string
	Force      bool

	LogLevel    string
	HexapiLevel string
	LogFormat   string
}

// Flags is the optional CLI-flag overlay. Fields that are zero-valued are
// ignored; non-zero values override env vars. Since 1.4.0.
type Flags struct {
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	OutputDir  string
	ModulePath string
	Force      bool

	LogLevel    string
	HexapiLevel string
	LogFormat   string
}

// has reports whether a string flag was supplied (non-empty). Since 1.4.0.
func (f Flags) has(v string) bool { return v != "" }

// hasPort reports whether the port flag was supplied (non-zero). Since 1.4.0.
func (f Flags) hasPort() bool { return f.DBPort != 0 }

// getenv is the env-var accessor signature used by ToSettings. Since 1.4.0.
type getenv func(string) string

// ToSettings builds a Settings from env vars (via getenv) overlaid with any
// non-zero CLI flags. It validates required fields and parses ints; on any
// failure it logs the cause at ERROR and returns a typed GenError (GEN-001).
//
// Since 1.4.0.
func ToSettings(log *applogger.Logger, env getenv, f Flags) (Settings, error) {
	return applogger.Logged1(log, "Settings.ToSettings", func() (Settings, error) {
		s := Settings{
			DBHost:      pickString(f.has(f.DBHost), f.DBHost, env("POSTGRES_HOST"), "127.0.0.1"),
			DBUser:      pickString(f.has(f.DBUser), f.DBUser, env("POSTGRES_USER"), "hexapi"),
			DBPassword:  pickString(f.has(f.DBPassword), f.DBPassword, env("POSTGRES_PASSWORD"), ""),
			DBName:      pickString(f.has(f.DBName), f.DBName, env("POSTGRES_DB"), "hexapi_db"),
			OutputDir:   pickString(f.has(f.OutputDir), f.OutputDir, env("GENERATOR_OUTPUT_DIR"), "./out"),
			ModulePath:  pickString(f.has(f.ModulePath), f.ModulePath, env("GENERATOR_MODULE_PATH"), "github.com/example/generated-api"),
			Force:       f.Force,
			LogLevel:    pickString(f.has(f.LogLevel), f.LogLevel, env("LOG_LEVEL"), "INFO"),
			HexapiLevel: pickString(f.has(f.HexapiLevel), f.HexapiLevel, env("HEXAPI_LOG_LEVEL"), "DEBUG"),
			LogFormat:   pickString(f.has(f.LogFormat), f.LogFormat, env("LOG_FORMAT"), "text"),
		}

		// Port: prefer flag, then env, then default 5432.
		portStr := env("POSTGRES_PORT")
		switch {
		case f.hasPort():
			s.DBPort = f.DBPort
		case portStr != "":
			p, err := strconv.Atoi(portStr)
			if err != nil {
				// P0/v1.4.0: log the cause before rethrowing as a typed error.
				log.Error("Settings.ToSettings: invalid POSTGRES_PORT", "err", err, "value", portStr)
				return Settings{}, errors.Wrap(errors.CodeConfig, "POSTGRES_PORT is not an integer", err)
			}
			s.DBPort = p
		default:
			s.DBPort = 5432
		}

		if err := s.validate(log); err != nil {
			return Settings{}, err
		}
		return s, nil
	})
}

// pickString selects the first non-empty value, falling back to def.
// Since 1.4.0.
func pickString(flagSet bool, flagVal, envVal, def string) string {
	switch {
	case flagSet && flagVal != "":
		return flagVal
	case envVal != "":
		return envVal
	default:
		return def
	}
}

// validate enforces required fields and range constraints. It logs the cause
// at ERROR before returning a typed GenError. Since 1.4.0.
func (s Settings) validate(log *applogger.Logger) error {
	if s.DBPassword == "" {
		log.Error("Settings.validate: POSTGRES_PASSWORD missing")
		return errors.New(errors.CodeConfig, "POSTGRES_PASSWORD is required (set it in .env)")
	}
	if s.DBUser == "" {
		log.Error("Settings.validate: POSTGRES_USER empty")
		return errors.New(errors.CodeConfig, "POSTGRES_USER is required")
	}
	if s.DBName == "" {
		log.Error("Settings.validate: POSTGRES_DB empty")
		return errors.New(errors.CodeConfig, "POSTGRES_DB is required")
	}
	if s.DBPort < 1 || s.DBPort > 65535 {
		log.Error("Settings.validate: POSTGRES_PORT out of range", "port", s.DBPort)
		return errors.New(errors.CodeConfig, "POSTGRES_PORT must be in 1..65535")
	}
	if s.OutputDir == "" {
		log.Error("Settings.validate: output dir empty")
		return errors.New(errors.CodeConfig, "output directory is required")
	}
	if s.ModulePath == "" {
		log.Error("Settings.validate: module path empty")
		return errors.New(errors.CodeConfig, "module path is required")
	}
	if lf := s.LogFormat; lf != "text" && lf != "json" {
		log.Error("Settings.validate: bad LOG_FORMAT", "value", lf)
		return errors.New(errors.CodeConfig, "LOG_FORMAT must be 'text' or 'json'")
	}
	return nil
}

// DSN returns the lib/pq connection string. Since 1.4.0.
func (s Settings) DSN() string {
	return "host=" + s.DBHost +
		" port=" + strconv.Itoa(s.DBPort) +
		" user=" + s.DBUser +
		" password=" + s.DBPassword +
		" dbname=" + s.DBName +
		" sslmode=disable"
}
