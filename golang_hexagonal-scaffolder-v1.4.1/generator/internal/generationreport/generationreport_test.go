package generationreport

import (
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
)

func newBuilder() *Builder { return New(applogger.NewWithEnv(func(string) string { return "" })) }

// TestReport_Add_happy covers adding files. Since 1.4.0.
func TestReport_Add_happy(t *testing.T) {
	b := newBuilder()
	if err := b.Add("a.go", "package a"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := b.Add("b.go", "package b"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	r, err := b.Finalise()
	if err != nil {
		t.Fatalf("Finalise: %v", err)
	}
	if r.FileCount() != 2 {
		t.Fatalf("FileCount: want 2, got %d", r.FileCount())
	}
	if r.TotalBytes() != len("package a")+len("package b") {
		t.Fatalf("TotalBytes: want %d, got %d", len("package a")+len("package b"), r.TotalBytes())
	}
}

// TestReport_Add_emptyPath covers the GEN-004 path. Since 1.4.0.
func TestReport_Add_emptyPath(t *testing.T) {
	b := newBuilder()
	err := b.Add("", "x")
	if err == nil || !strings.Contains(err.Error(), "GEN-004") {
		t.Fatalf("Add empty: want GEN-004, got %v", err)
	}
}

// TestReport_Finalise_empty covers the no-files path. Since 1.4.0.
func TestReport_Finalise_empty(t *testing.T) {
	b := newBuilder()
	_, err := b.Finalise()
	if err == nil || !strings.Contains(err.Error(), "GEN-004") {
		t.Fatalf("Finalise empty: want GEN-004, got %v", err)
	}
}

// TestReport_Summary_containsFileAndDir covers the human summary. Since 1.4.0.
func TestReport_Summary_containsFileAndDir(t *testing.T) {
	b := newBuilder()
	b.SetOutputDir("./out")
	_ = b.Add("a.go", "package a")
	r, _ := b.Finalise()
	s := r.Summary()
	if !strings.Contains(s, "a.go") || !strings.Contains(s, "./out") {
		t.Fatalf("Summary missing file or dir: %s", s)
	}
}

// TestReport_SetModule_persists covers the module path. Since 1.4.0.
func TestReport_SetModule_persists(t *testing.T) {
	b := newBuilder()
	b.SetModule("github.com/example/x")
	b.SetOutputDir("./out")
	_ = b.Add("a.go", "x")
	r, _ := b.Finalise()
	if r.ModulePath != "github.com/example/x" {
		t.Fatalf("ModulePath: want github.com/example/x, got %q", r.ModulePath)
	}
}
