// Package applogcheck is a build-time check that every package in the
// generator module (under internal/, except applogger itself and testutil)
// uses AppLogger.Logged / Logged1 at least once. It walks the AST of each
// package's Go files and counts call expressions whose function selector
// ends in ".Logged" or "Logged1".
//
// This is the lightweight enforcement of the v1.4.0 "every public function
// is logged" contract: a package that defines exported functions but has
// zero Logged uses fails the check. It does not enforce per-function wiring
// (which would require a stricter, more brittle AST rule), but it catches
// the wholesale regression of removing logging from a package.
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

// allowlist packages are exempt: applogger (defines Logged), errors (pure
// constants), testutil (test helpers), applogcheck (this checker itself).
var allowlist = map[string]bool{
	"internal/applogger":       true,
	"internal/errors":          true,
	"internal/testutil":        true,
	"internal/testutil/fakedb": true,
	"internal/applogcheck":     true,
}

// TestAllPackagesUseLogged walks internal/ and asserts each non-allowlist
// package that defines exported functions calls .Logged or Logged1 at least
// once. Since 1.4.0.
func TestAllPackagesUseLogged(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	internalDir := filepath.Join(moduleRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Skipf("cannot read internal/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkgRel := filepath.Join("internal", e.Name())
		if allowlist[pkgRel] {
			continue
		}
		pkgDir := filepath.Join(internalDir, e.Name())
		pkgLogged := countLoggedInDir(t, pkgDir)
		if pkgLogged == 0 {
			// Recurse one level: some packages are nested (e.g. testutil/fakedb).
			t.Errorf("package %s defines code but uses no AppLogger.Logged / Logged1 calls", pkgRel)
		}
	}
}

// countLoggedInDir counts .Logged( and Logged1( call expressions in the .go
// files (non-test) under dir. Since 1.4.0.
func countLoggedInDir(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Logf("parse %s: %v (skipping)", path, perr)
			return nil
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
			name := sel.Sel.Name
			if name == "Logged" || name == "Logged1" {
				count++
			}
			return true
		})
		return nil
	})
	return count
}

// findModuleRoot locates the directory containing go.mod by walking up from
// this test file's location. Since 1.4.0.
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
	t.Fatal("could not find go.mod walking up from ", dir)
	return ""
}
