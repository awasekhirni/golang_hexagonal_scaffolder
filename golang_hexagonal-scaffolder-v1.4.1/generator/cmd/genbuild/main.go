// gen_and_build.go: driver that generates a project from a fake schema and
// prints the output dir, so we can run `go build` on it externally.
// Since 1.4.0.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/generator"
	"github.com/example/hexapi/generator/internal/schemaintrospector"
	"github.com/example/hexapi/generator/internal/settings"
)

func main() {
	out := os.Args[1]
	log := applogger.NewWithEnv(func(string) string { return "" })
	s := settings.Settings{
		OutputDir:  out,
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
		},
	}
	provide := func(ctx context.Context) (*schemaintrospector.Schema, error) { return schema, nil }
	g := generator.New(log, s, provide)
	td, _ := filepath.Abs("templates")
	report, err := g.Generate(context.Background(), td)
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
	fmt.Println(report.Summary())
}
