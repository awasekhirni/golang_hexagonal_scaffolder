package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/schemaintrospector"
	"github.com/example/hexapi/generator/internal/settings"
)

func log() *applogger.Logger { return applogger.NewWithEnv(func(string) string { return "" }) }

func baseSettings(t *testing.T) (settings.Settings, string) {
	t.Helper()
	dir := t.TempDir()
	return settings.Settings{
		OutputDir:  dir,
		ModulePath: "github.com/example/gen",
		Force:      true,
	}, dir
}

func fakeSchema() *schemaintrospector.Schema {
	return &schemaintrospector.Schema{
		Name: "public",
		Tables: []schemaintrospector.Table{
			{Name: "users", Columns: []schemaintrospector.Column{
				{Name: "id", DataType: "uuid", Required: true},
				{Name: "email", DataType: "text", Required: true},
				{Name: "created_at", DataType: "timestamptz"},
			}},
		},
	}
}

func templateDir(t *testing.T) string {
	t.Helper()
	// Templates live in <repo>/generator/templates. Walk up from this test's
	// working directory to find them.
	candidates := []string{"../../templates", "templates", "../../../templates"}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				return abs
			}
		}
	}
	t.Skipf("templates dir not found; skipping generator end-to-end test")
	return ""
}

// TestGenerate_endToEnd covers the full pipeline with a fake schema provider.
// Since 1.4.0.
func TestGenerate_endToEnd(t *testing.T) {
	s, dir := baseSettings(t)
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return fakeSchema(), nil }
	g := New(log(), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	report, err := g.Generate(context.Background(), td)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if report.FileCount() == 0 {
		t.Fatalf("no files generated")
	}
	// The entity file should exist for the users table.
	entityPath := filepath.Join(dir, "internal/domain/users/users.go")
	if _, err := os.Stat(entityPath); err != nil {
		t.Fatalf("entity file missing: %v", err)
	}
}

// TestGenerate_nonEmptyWithoutForce covers the GEN-006 path. Since 1.4.0.
func TestGenerate_nonEmptyWithoutForce(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a file so the dir is non-empty.
	if err := os.WriteFile(filepath.Join(dir, "preexisting"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := settings.Settings{OutputDir: dir, ModulePath: "m", Force: false}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return fakeSchema(), nil }
	g := New(log(), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	_, err := g.Generate(context.Background(), td)
	if err == nil || !strings.Contains(err.Error(), "GEN-006") {
		t.Fatalf("non-empty without force: want GEN-006, got %v", err)
	}
}

// TestGenerate_forceOverwrites covers the --force path. Since 1.4.0.
func TestGenerate_forceOverwrites(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "preexisting"), []byte("x"), 0o644)
	s := settings.Settings{OutputDir: dir, ModulePath: "m", Force: true}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return fakeSchema(), nil }
	g := New(log(), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	if _, err := g.Generate(context.Background(), td); err != nil {
		t.Fatalf("Generate with force: %v", err)
	}
}

// TestGenerate_schemaProviderFails covers the error pass-through from the
// provider. Since 1.4.0.
func TestGenerate_schemaProviderFails(t *testing.T) {
	s, _ := baseSettings(t)
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) {
		return nil, schemaintrospectorErr("GEN-003: introspect failed")
	}
	g := New(log(), s, provide)
	td := templateDir(t)
	if td == "" {
		return
	}
	_, err := g.Generate(context.Background(), td)
	if err == nil || !strings.Contains(err.Error(), "GEN-003") {
		t.Fatalf("provider fail: want GEN-003, got %v", err)
	}
}

// TestGenerate_buildModel covers the model builder directly. Since 1.4.0.
func TestGenerate_buildModel(t *testing.T) {
	s, _ := baseSettings(t)
	g := New(log(), s, func(ctx context.Context) (*schemaintrospector.Schema, error) { return nil, nil })
	m, err := g.buildModel(fakeSchema())
	if err != nil {
		t.Fatalf("buildModel: %v", err)
	}
	if len(m.Tables) != 1 || m.Tables[0].EntityName != "User" {
		t.Fatalf("model: %+v", m.Tables)
	}
	if m.Tables[0].RouteName != "users" {
		t.Fatalf("route name: want users, got %q", m.Tables[0].RouteName)
	}
}

// schemaintrospectorErr is a stand-in typed error for the provider-fail test.
type schemaintrospectorErr string

func (e schemaintrospectorErr) Error() string { return string(e) }
func (e schemaintrospectorErr) Code() string  { return "GEN-003" }
