// Package applogcheck enforces the "every package uses AppLogger.Logged /
// Logged1" contract by walking the AST of each package under internal/.
//
// Since 1.4.0.
package applogcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowlist packages are exempt (they define Logged, are pure constants, are
// test helpers, or are HTTP middleware whose handler-returning functions do
// not fit the error-returning Logged pattern). Since 1.4.0.
var allowlist = map[string]bool{
	"internal/shared/applogger": true,
	"internal/shared/errors":    true,
	"internal/testutil":         true,
	"internal/testutil/fakedb":  true,
	"internal/applogcheck":      true,
	"internal/app/server":       true, // wiring; calls into Logged packages
	"internal/app/middleware":   true, // HTTP handlers return http.HandlerFunc, not error
	"internal/app/routes":       true, // HTTP handlers return via WriteError, not Logged
}

// TestAllPackagesUseLogged walks every directory under internal/ that
// contains non-test .go files, and asserts each non-allowlisted package uses
// .Logged or Logged1 at least once. Since 1.4.0.
func TestAllPackagesUseLogged(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	internalDir := filepath.Join(moduleRoot, "internal")
	packages := findGoPackages(t, internalDir)
	for _, pkg := range packages {
		rel := filepath.ToSlash(pkg.rel)
		if allowlist[rel] {
			continue
		}
		count := countLoggedInDir(t, pkg.dir)
		if count == 0 {
			t.Errorf("package %s defines code but uses no AppLogger.Logged / Logged1 calls", rel)
		}
	}
}

// pkgEntry is a directory containing non-test .go files. Since 1.4.0.
type pkgEntry struct {
	dir string
	rel string // relative to module root
}

// findGoPackages walks dir and returns every directory containing non-test
// .go files. Since 1.4.0.
func findGoPackages(t *testing.T, dir string) []pkgEntry {
	t.Helper()
	var out []pkgEntry
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		hasGo := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			return nil
		}
		rel, err := filepath.Rel(findModuleRoot(t), path)
		if err != nil {
			return nil
		}
		out = append(out, pkgEntry{dir: path, rel: rel})
		return nil
	})
	return out
}

// countLoggedInDir counts .Logged( and Logged1( call expressions in the
// non-test .go files under dir. Since 1.4.0.
func countLoggedInDir(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Logf("parse %s: %v (skipping)", path, perr)
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Logged" || sel.Sel.Name == "Logged1" {
				count++
			}
			return true
		})
	}
	return count
}

// findModuleRoot locates go.mod by walking up. Since 1.4.0.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find go.mod")
	return ""
}
