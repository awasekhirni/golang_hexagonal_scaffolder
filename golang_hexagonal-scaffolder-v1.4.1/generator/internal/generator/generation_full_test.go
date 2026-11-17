// generator_test.go additions: comprehensive end-to-end generation that
// asserts the full file set is emitted and that every package imported by
// the generated main.go resolves to a generated file. Since 1.4.0.
package generator

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/schemaintrospector"
	"github.com/example/hexapi/generator/internal/settings"
)

// TestGenerate_fullProject_emitsAllFiles drives the generator with a
// two-table fake schema and asserts every expected artefact is written.
// Since 1.4.0.
func TestGenerate_fullProject_emitsAllFiles(t *testing.T) {
	dir := t.TempDir()
	s := settings.Settings{
		OutputDir:  dir,
		ModulePath: "github.com/example/out",
		Force:      true,
	}
	schema := &schemaintrospector.Schema{
		Name: "public",
		Tables: []schemaintrospector.Table{
			{Name: "users", Columns: []schemaintrospector.Column{
				{Name: "id", DataType: "uuid", Required: true},
				{Name: "email", DataType: "text", Required: true},
				{Name: "name", DataType: "text", Required: true},
				{Name: "created_at", DataType: "timestamptz"},
			}},
			{Name: "orders", Columns: []schemaintrospector.Column{
				{Name: "id", DataType: "uuid", Required: true},
				{Name: "user_id", DataType: "uuid", Required: true},
				{Name: "total", DataType: "numeric"},
			}},
		},
	}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return schema, nil }
	g := New(applogger.NewWithEnv(func(string) string { return "" }), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	report, err := g.Generate(context.Background(), td)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Expected files for a two-table schema.
	want := []string{
		"go.mod",
		"README.md",
		"cmd/api/main.go",
		"Dockerfile",
		".env.example",
		"application.yaml",
		"resources/openapi.yaml",
		"resources/migrations/0001_init.sql",
		// infrastructure
		"internal/shared/applogger/applogger.go",
		"internal/shared/errors/errors.go",
		"internal/config/settings/settings.go",
		"internal/infra/database/databasefactory.go",
		"internal/infra/bus/bus.go",
		"internal/app/middleware/middleware.go",
		"internal/app/middleware/crypto.go",
		"internal/app/routes/routes.go",
		"internal/app/routes/auth.go",
		"internal/app/server/server.go",
		"internal/app/server/helpers.go",
		// per-table: users
		"internal/domain/users/users.go",
		"internal/domain/users/repository/repository.go",
		"internal/application/users/service.go",
		"internal/application/users/controller/controller.go",
		// per-table: orders
		"internal/domain/orders/orders.go",
		"internal/domain/orders/repository/repository.go",
		"internal/application/orders/service.go",
		"internal/application/orders/controller/controller.go",
	}
	for _, w := range want {
		full := filepath.Join(dir, w)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing expected file: %s (%v)", w, err)
		}
	}
	t.Logf("generated %d files", report.FileCount())

	// Verify main.go's imports resolve to generated files.
	mainGo, err := os.ReadFile(filepath.Join(dir, "cmd/api/main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	imports := extractImports(string(mainGo))
	for _, imp := range imports {
		if !strings.HasPrefix(imp, "github.com/example/out/") {
			continue // skip stdlib + third-party
		}
		rel := strings.TrimPrefix(imp, "github.com/example/out/")
		full := filepath.Join(dir, filepath.FromSlash(rel)+".go")
		// The import path may be a package dir; check for a .go file in it.
		if _, err := os.Stat(full); err != nil {
			// Try the directory — at least one .go file should exist.
			dirPath := filepath.Join(dir, filepath.FromSlash(rel))
			entries, derr := os.ReadDir(dirPath)
			if derr != nil || !hasGoFile(entries) {
				t.Errorf("main.go import %q does not resolve to a generated file: %s", imp, rel)
			}
		}
	}
}

// TestGenerate_migrationSQL_isIdempotent asserts the generated migration
// uses CREATE TABLE IF NOT EXISTS. Since 1.4.0.
func TestGenerate_migrationSQL_isIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := settings.Settings{OutputDir: dir, ModulePath: "github.com/example/out", Force: true}
	schema := &schemaintrospector.Schema{
		Name: "public",
		Tables: []schemaintrospector.Table{
			{Name: "users", Columns: []schemaintrospector.Column{
				{Name: "id", DataType: "uuid", Required: true},
				{Name: "email", DataType: "text", Required: true},
			}},
		},
	}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return schema, nil }
	g := New(applogger.NewWithEnv(func(string) string { return "" }), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	if _, err := g.Generate(context.Background(), td); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sql, err := os.ReadFile(filepath.Join(dir, "resources/migrations/0001_init.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if !strings.Contains(string(sql), "CREATE TABLE IF NOT EXISTS") {
		t.Errorf("migration should use CREATE TABLE IF NOT EXISTS:\n%s", sql)
	}
	if !strings.Contains(string(sql), `"users"`) {
		t.Errorf("migration should reference the users table:\n%s", sql)
	}
}

// TestGenerate_openapi_hasPathsForEveryTable. Since 1.4.0.
func TestGenerate_openapi_hasPathsForEveryTable(t *testing.T) {
	dir := t.TempDir()
	s := settings.Settings{OutputDir: dir, ModulePath: "github.com/example/out", Force: true}
	schema := &schemaintrospector.Schema{
		Name: "public",
		Tables: []schemaintrospector.Table{
			{Name: "users", Columns: []schemaintrospector.Column{{Name: "id", DataType: "uuid", Required: true}}},
			{Name: "orders", Columns: []schemaintrospector.Column{{Name: "id", DataType: "uuid", Required: true}}},
		},
	}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return schema, nil }
	g := New(applogger.NewWithEnv(func(string) string { return "" }), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	if _, err := g.Generate(context.Background(), td); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	spec, err := os.ReadFile(filepath.Join(dir, "resources/openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	body := string(spec)
	for _, route := range []string{"/users", "/orders"} {
		if !strings.Contains(body, route+":") {
			t.Errorf("openapi missing path %s", route)
		}
	}
}

// extractImports returns the import paths from a Go source string. Since 1.4.0.
func extractImports(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"`) {
			out = append(out, strings.Trim(line, `"`))
		}
	}
	sort.Strings(out)
	return out
}

// hasGoFile reports whether any entry is a .go file. Since 1.4.0.
func hasGoFile(entries []os.DirEntry) bool {
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}
