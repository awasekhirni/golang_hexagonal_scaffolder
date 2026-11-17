// Package main is the entry point for the generator CLI.
//
// Usage:
//
//	go run ./cmd/generator \
//	  --db-host 127.0.0.1 --db-port 5432 \
//	  --db-user hexapi --db-password "$POSTGRES_PASSWORD" \
//	  --db-name hexapi_db \
//	  --out ../out --module github.com/example/generated-api --force
//
// Flags overlay env vars; env vars overlay defaults from application.yaml.
// See generator/README.md for the full matrix.
//
// Since 1.4.0.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/databasefactory"
	"github.com/example/hexapi/generator/internal/errors"
	"github.com/example/hexapi/generator/internal/generator"
	"github.com/example/hexapi/generator/internal/schemaintrospector"
	"github.com/example/hexapi/generator/internal/settings"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "generator:", err)
		os.Exit(1)
	}
}

// run is the testable core of main. It parses flags, builds settings,
// opens the DB, introspects, generates, and writes the report. Since 1.4.0.
func run(args []string, env func(string) string, stdout *os.File) error {
	log := applogger.NewWithEnv(env)

	fs := flag.NewFlagSet("generator", flag.ContinueOnError)
	fs.SetOutput(stdout)
	var f settings.Flags
	fs.StringVar(&f.DBHost, "db-host", "", "PostgreSQL host (env POSTGRES_HOST)")
	fs.IntVar(&f.DBPort, "db-port", 0, "PostgreSQL port (env POSTGRES_PORT)")
	fs.StringVar(&f.DBUser, "db-user", "", "PostgreSQL user (env POSTGRES_USER)")
	fs.StringVar(&f.DBPassword, "db-password", "", "PostgreSQL password (env POSTGRES_PASSWORD, required)")
	fs.StringVar(&f.DBName, "db-name", "", "PostgreSQL database name (env POSTGRES_DB)")
	fs.StringVar(&f.OutputDir, "out", "", "output directory (env GENERATOR_OUTPUT_DIR)")
	fs.StringVar(&f.ModulePath, "module", "", "Go module path for generated go.mod (env GENERATOR_MODULE_PATH)")
	fs.BoolVar(&f.Force, "force", false, "overwrite a non-empty output directory")
	fs.StringVar(&f.LogLevel, "log-level", "", "root log level (env LOG_LEVEL)")
	fs.StringVar(&f.HexapiLevel, "hexapi-log-level", "", "hexapi subtree log level (env HEXAPI_LOG_LEVEL)")
	fs.StringVar(&f.LogFormat, "log-format", "", "log format: text|json (env LOG_FORMAT)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 || (fs.NArg() == 1 && fs.Arg(0) == "help") {
		// Allow `--help` or `help`.
		if helpRequested(args) {
			fs.Usage()
			return nil
		}
	}

	s, err := settings.ToSettings(log, env, f)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db := databasefactory.New(log)
	if err := db.Init(ctx, databasefactory.DriverName, s.DSN()); err != nil {
		return err
	}
	defer db.Close()

	// Provide the schema via the introspector.
	intro := schemaintrospector.New(log, db.DB())
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) {
		return intro.Introspect(ctx, "public")
	}

	// Templates ship alongside the binary. Resolve relative to the module root.
	templateDir, err := resolveTemplateDir()
	if err != nil {
		return errors.Wrap(errors.CodeTemplate, "resolve template dir failed", err)
	}

	gen := generator.New(log, s, provide)
	report, err := gen.Generate(ctx, templateDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, report.Summary())
	return nil
}

// helpRequested reports whether the user passed --help or -h. Since 1.4.0.
func helpRequested(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return true
		}
	}
	return false
}

// resolveTemplateDir finds the templates directory. It prefers
// $GENERATOR_TEMPLATES (for tests), then ./templates relative to the working
// directory, then ./generator/templates (when run from the repo root).
// Since 1.4.0.
func resolveTemplateDir() (string, error) {
	if v := os.Getenv("GENERATOR_TEMPLATES"); v != "" {
		return v, nil
	}
	candidates := []string{"templates", "generator/templates", "../templates"}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("templates directory not found (tried %v); set GENERATOR_TEMPLATES", candidates)
}
