// Package generator orchestrates the generation pipeline:
//
//	introspect → name → render → write → report
//
// Every step runs under AppLogger.Logged; every error is logged at ERROR
// with stack-friendly attributes before being re-wrapped as a typed GEN-*
// error.
//
// Since 1.4.0.
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
	"github.com/example/hexapi/generator/internal/generationreport"
	"github.com/example/hexapi/generator/internal/naming"
	"github.com/example/hexapi/generator/internal/schemaintrospector"
	"github.com/example/hexapi/generator/internal/settings"
	"github.com/example/hexapi/generator/internal/templateengine"
)

// SchemaProvider returns the introspected schema. The Generator accepts this
// as a function so tests can inject a fake without standing up Postgres.
// Since 1.4.0.
type SchemaProvider func(ctx context.Context) (*schemaintrospector.Schema, error)

// Generator is the pipeline orchestrator. Since 1.4.0.
type Generator struct {
	log     *applogger.Logger
	s       settings.Settings
	naming  *naming.Naming
	engine  *templateengine.Engine
	provide SchemaProvider
	report  *generationreport.Builder
}

// New constructs a Generator. Since 1.4.0.
func New(log *applogger.Logger, s settings.Settings, provide SchemaProvider) *Generator {
	if log == nil {
		log = applogger.New()
	}
	n := naming.New(log)
	e := templateengine.New(log, n)
	return &Generator{
		log:     log,
		s:       s,
		naming:  n,
		engine:  e,
		provide: provide,
		report:  generationreport.New(log),
	}
}

// Generate runs the pipeline and returns the final report.
// Steps: load templates, introspect, build model, render, write, finalise.
// Since 1.4.0.
func (g *Generator) Generate(ctx context.Context, templateDir string) (*generationreport.Report, error) {
	return applogger.Logged1(g.log, "Generator.Generate", func() (*generationreport.Report, error) {
		g.report.SetModule(g.s.ModulePath)
		g.report.SetOutputDir(g.s.OutputDir)

		if err := g.loadTemplates(templateDir); err != nil {
			return nil, err
		}
		schema, err := g.provide(ctx)
		if err != nil {
			return nil, err
		}
		model, err := g.buildModel(schema)
		if err != nil {
			return nil, err
		}
		rendered, err := g.renderAll(model)
		if err != nil {
			return nil, err
		}
		if err := g.writeFiles(rendered); err != nil {
			return nil, err
		}
		return g.report.Finalise()
	})
}

// loadTemplates parses every .tmpl file in templateDir. Since 1.4.0.
func (g *Generator) loadTemplates(templateDir string) error {
	return g.log.Logged("Generator.loadTemplates", func() error {
		entries, err := os.ReadDir(templateDir)
		if err != nil {
			g.log.Error("loadTemplates: read dir failed", "dir", templateDir, "err", err)
			return errors.Wrap(errors.CodeTemplate, "read template dir failed", err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".tmpl" {
				continue
			}
			src, err := os.ReadFile(filepath.Join(templateDir, e.Name()))
			if err != nil {
				g.log.Error("loadTemplates: read file failed", "file", e.Name(), "err", err)
				return errors.Wrap(errors.CodeTemplate, "read template file failed: "+e.Name(), err)
			}
			name := e.Name()[:len(e.Name())-len(".tmpl")]
			if err := g.engine.Parse(name, string(src)); err != nil {
				return err
			}
		}
		return nil
	})
}

// renderModel is the data passed to templates. Since 1.4.0.
type renderModel struct {
	ModulePath string
	SchemaName string
	Tables     []tableModel
}

// tableModel is one table's render-time view. Since 1.4.0.
type tableModel struct {
	Table           string
	EntityName      string
	RouteName       string
	Package         string
	Columns         []columnModel
	ColumnsList     string
	UpdateSet       string // "col1" = $1, "col2" = $2, ...
	UpdateArgs      string // e.Col1, e.Col2, ..., e.ID
	WherePlaceholder int   // the $N for WHERE id = $N
	MigrationSQL    string // CREATE TABLE IF NOT EXISTS ... derived from schema
	IDGoName        string // GoName of the primary-key column (default "ID")
}

// columnModel is one column's render-time view. Since 1.4.0.
type columnModel struct {
	Name      string
	GoName    string
	GoType    string
	Required  bool
	ZeroValue string
}

// buildModel converts the introspected schema into the render model.
// Since 1.4.0.
func (g *Generator) buildModel(schema *schemaintrospector.Schema) (*renderModel, error) {
	return applogger.Logged1(g.log, "Generator.buildModel", func() (*renderModel, error) {
		m := &renderModel{
			ModulePath: g.s.ModulePath,
			SchemaName: schema.Name,
		}
		tables := append([]schemaintrospector.Table(nil), schema.Tables...)
		sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })
		for _, t := range tables {
			ent, err := g.naming.EntityName(t.Name)
			if err != nil {
				return nil, err
			}
			route, err := g.naming.RouteName(t.Name)
			if err != nil {
				return nil, err
			}
			pkg, err := g.naming.Sanitise(t.Name)
			if err != nil {
				return nil, err
			}
			tm := tableModel{
				Table:      t.Name,
				EntityName: ent,
				RouteName:  route,
				Package:    pkg,
			}
			var colNames []string
			for _, c := range t.Columns {
				gn, err := g.naming.GoName(c.Name)
				if err != nil {
					return nil, err
				}
				tm.Columns = append(tm.Columns, columnModel{
					Name:      c.Name,
					GoName:    gn,
					GoType:    goTypeFor(c.DataType),
					Required:  c.Required,
					ZeroValue: zeroValueFor(goTypeFor(c.DataType)),
				})
				colNames = append(colNames, `"`+c.Name+`"`)
			}
			tm.ColumnsList = joinCSV(colNames)
			tm.UpdateSet = updateSet(tm.Columns)
			tm.UpdateArgs = updateArgs(tm.Columns)
			tm.WherePlaceholder = len(t.Columns) + 1
			tm.IDGoName = idGoName(tm.Columns)
			tm.MigrationSQL = migrationSQLFor(t)
			m.Tables = append(m.Tables, tm)
		}
		return m, nil
	})
}

// renderAll renders every parsed template and routes the output to a path.
// Per-table templates (entity/repository/service/controller) are expanded
// once per table; all other templates render once and are routed via
// oncePath. Since 1.4.0.
func (g *Generator) renderAll(m *renderModel) (map[string]string, error) {
	return applogger.Logged1(g.log, "Generator.renderAll", func() (map[string]string, error) {
		out := map[string]string{}
		for _, name := range g.engine.Names() {
			switch name {
			case "entity.go", "repository.go", "service.go", "controller.go":
				// Per-table templates: render once per table with a flat model.
				for _, t := range m.Tables {
					data := perTableData{
						ModulePath:       m.ModulePath,
						SchemaName:       m.SchemaName,
						Package:          t.Package,
						Table:            t.Table,
						EntityName:       t.EntityName,
						RouteName:        t.RouteName,
						Columns:          t.Columns,
						ColumnsList:      t.ColumnsList,
						Placeholders:     placeholders(len(t.Columns)),
						ScanArgs:         scanArgs(t.Columns),
						SaveArgs:         saveArgs(t.Columns),
						UpdateSet:        t.UpdateSet,
						UpdateArgs:       t.UpdateArgs,
						WherePlaceholder: t.WherePlaceholder,
						MigrationSQL:     t.MigrationSQL,
						IDGoName:         t.IDGoName,
					}
					rendered, err := g.engine.Execute(name, data)
					if err != nil {
						return nil, err
					}
					path := perTablePath(name, t)
					out[path] = rendered
				}
			default:
				rendered, err := g.engine.Execute(name, m)
				if err != nil {
					return nil, err
				}
				path, ok := oncePath(name)
				if !ok {
					path = name
				}
				out[path] = rendered
			}
		}
		return out, nil
	})
}

// oncePath maps a once-only template name to its output path within the
// generated project. Templates not listed here use their own name as the path.
// Since 1.4.0.
func oncePath(name string) (string, bool) {
	paths := map[string]string{
		"go.mod":              "go.mod",
		"README.md":           "README.md",
		"main.go":             "cmd/api/main.go",
		"Dockerfile":          "Dockerfile",
		"env.example":         ".env.example",
		"application.yaml":    "application.yaml",
		"openapi.yaml":        "resources/openapi.yaml",
		"migration.sql":       "resources/migrations/0001_init.sql",
		"infra_applogger.go":  "internal/shared/applogger/applogger.go",
		"infra_errors.go":     "internal/shared/errors/errors.go",
		"infra_settings.go":   "internal/config/settings/settings.go",
		"infra_database.go":   "internal/infra/database/databasefactory.go",
		"infra_bus.go":        "internal/infra/bus/bus.go",
		"infra_middleware.go": "internal/app/middleware/middleware.go",
		"infra_crypto.go":     "internal/app/middleware/crypto.go",
		"infra_routes.go":     "internal/app/routes/routes.go",
		"infra_auth.go":       "internal/app/routes/auth.go",
		"server.go":           "internal/app/server/server.go",
		"helpers.go":          "internal/app/server/helpers.go",
	}
	p, ok := paths[name]
	return p, ok
}

// perTableData is the flat model for per-table templates. It carries every
// field the entity/repository/service/controller templates need. Since 1.4.0.
type perTableData struct {
	ModulePath       string
	SchemaName       string
	Package          string
	Table            string
	EntityName       string
	RouteName        string
	Columns          []columnModel
	ColumnsList      string
	Placeholders     string
	ScanArgs         string
	SaveArgs         string
	UpdateSet        string
	UpdateArgs       string
	WherePlaceholder int
	MigrationSQL     string
	IDGoName         string
}

// placeholders returns "$1, $2, ..." for n columns. Since 1.4.0.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := "$1"
	for i := 2; i <= n; i++ {
		out += fmt.Sprintf(", $%d", i)
	}
	return out
}

// scanArgs returns "e.Field1, e.Field2, ..." for use in rows.Scan. Since 1.4.0.
func scanArgs(cols []columnModel) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, "&e."+c.GoName)
	}
	return joinCSV(parts)
}

// saveArgs returns "e.Field1, e.Field2, ..." for INSERT args. Since 1.4.0.
func saveArgs(cols []columnModel) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, "e."+c.GoName)
	}
	return joinCSV(parts)
}

// updateSet returns `"col1" = $1, "col2" = $2, ...` for the UPDATE SET clause.
// All columns are set (the id column is set to itself, which is harmless).
// Since 1.4.0.
func updateSet(cols []columnModel) string {
	var parts []string
	for i, c := range cols {
		parts = append(parts, fmt.Sprintf(`"%s" = $%d`, c.Name, i+1))
	}
	return joinCSV(parts)
}

// updateArgs returns `e.Field1, e.Field2, ..., e.ID` — all column values
// followed by the id for the WHERE clause. Since 1.4.0.
func updateArgs(cols []columnModel) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, "e."+c.GoName)
	}
	parts = append(parts, "e."+idGoName(cols))
	return joinCSV(parts)
}

// idGoName returns the GoName of the primary-key column: the column named "id"
// (case-insensitive), or the first column. Since 1.4.0.
func idGoName(cols []columnModel) string {
	for _, c := range cols {
		if strings.EqualFold(c.Name, "id") {
			return c.GoName
		}
	}
	if len(cols) > 0 {
		return cols[0].GoName
	}
	return "ID"
}

// migrationSQLFor builds an idempotent CREATE TABLE statement from the
// introspected table. The first column (or one named "id") is the PK.
// Since 1.4.0.
func migrationSQLFor(t schemaintrospector.Table) string {
	var b strings.Builder
	fmt.Fprintf(&b, `CREATE TABLE IF NOT EXISTS "%s" (`, t.Name)
	for i, c := range t.Columns {
		b.WriteString("\n    ")
		fmt.Fprintf(&b, `"%s" %s`, c.Name, c.DataType)
		if !c.Nullable {
			b.WriteString(" NOT NULL")
		}
		if i == 0 || strings.EqualFold(c.Name, "id") {
			b.WriteString(" PRIMARY KEY")
			if strings.EqualFold(c.Name, "id") {
				b.WriteString(" DEFAULT gen_random_uuid()")
			}
		}
		if i < len(t.Columns)-1 {
			b.WriteString(",")
		}
	}
	b.WriteString("\n);")
	return b.String()
}

// perTablePath returns the output path for a per-table file. Since 1.4.0.
func perTablePath(templateName string, t tableModel) string {
	switch templateName {
	case "entity.go":
		return fmt.Sprintf("internal/domain/%s/%s.go", t.Package, t.Package)
	case "repository.go":
		return fmt.Sprintf("internal/domain/%s/repository/repository.go", t.Package)
	case "service.go":
		return fmt.Sprintf("internal/application/%s/service.go", t.Package)
	case "controller.go":
		return fmt.Sprintf("internal/application/%s/controller/controller.go", t.Package)
	}
	return templateName + "/" + t.Package + ".go"
}

// writeFiles writes every rendered file under the output dir, enforcing the
// --force rule (GEN-006). Since 1.4.0.
func (g *Generator) writeFiles(files map[string]string) error {
	return g.log.Logged("Generator.writeFiles", func() error {
		if err := g.checkOutputDir(); err != nil {
			return err
		}
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			full := filepath.Join(g.s.OutputDir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				g.log.Error("writeFiles: mkdir failed", "path", filepath.Dir(full), "err", err)
				return errors.Wrap(errors.CodeGeneration, "mkdir failed: "+filepath.Dir(full), err)
			}
			if err := os.WriteFile(full, []byte(files[p]), 0o644); err != nil {
				g.log.Error("writeFiles: write failed", "path", full, "err", err)
				return errors.Wrap(errors.CodeGeneration, "write failed: "+full, err)
			}
			if err := g.report.Add(p, files[p]); err != nil {
				return err
			}
		}
		return nil
	})
}

// checkOutputDir enforces the --force rule. Since 1.4.0.
func (g *Generator) checkOutputDir() error {
	info, err := os.Stat(g.s.OutputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // will be created
		}
		g.log.Error("checkOutputDir: stat failed", "dir", g.s.OutputDir, "err", err)
		return errors.Wrap(errors.CodeGeneration, "stat output dir failed", err)
	}
	if !info.IsDir() {
		g.log.Error("checkOutputDir: not a directory", "dir", g.s.OutputDir)
		return errors.New(errors.CodeOutputDir, "output path is not a directory: "+g.s.OutputDir)
	}
	entries, err := os.ReadDir(g.s.OutputDir)
	if err != nil {
		return errors.Wrap(errors.CodeGeneration, "read output dir failed", err)
	}
	if len(entries) > 0 && !g.s.Force {
		g.log.Error("checkOutputDir: non-empty without --force", "dir", g.s.OutputDir, "count", len(entries))
		return errors.New(errors.CodeOutputDir, "output dir is non-empty; pass --force to overwrite")
	}
	return nil
}

// joinCSV joins strings with ", ". Since 1.4.0.
func joinCSV(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// goTypeFor maps an SQL type to a Go type (subset of the template engine's
// map; kept here so the model is self-contained for tests). Since 1.4.0.
func goTypeFor(sqlType string) string {
	switch sqlType {
	case "integer", "int", "int4", "serial":
		return "int"
	case "bigint", "int8", "bigserial":
		return "int64"
	case "smallint", "int2":
		return "int16"
	case "text", "varchar", "char", "uuid", "character varying":
		return "string"
	case "boolean", "bool":
		return "bool"
	case "timestamp", "timestamptz", "timestamp with time zone", "timestamp without time zone", "date":
		return "time.Time"
	case "numeric", "decimal", "float8", "double precision":
		return "float64"
	case "float4", "real":
		return "float32"
	case "jsonb", "json":
		return "json.RawMessage"
	}
	return "string"
}

// zeroValueFor returns the Go zero-literal for a Go type. Since 1.4.0.
func zeroValueFor(goType string) string {
	switch goType {
	case "int", "int16", "int32", "int64":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "float32", "float64":
		return "0.0"
	case "time.Time":
		return "time.Time{}"
	case "json.RawMessage":
		return "nil"
	}
	return "nil"
}
