// Package schemaintrospector reads a live PostgreSQL schema via
// information_schema and returns typed model objects the generator can render.
//
// v1.4.0 hardening (P1-3.9 / P1-3.4):
//   - QueryEnumValues uses a parameter-bound query ($1 placeholder) instead
//     of string interpolation. A regression test (TestQueryEnumValues_UsesBoundParameter)
//     asserts the SQL text contains exactly one "$1" and no concatenation.
//   - IsView and QueryEnumValues log the underlying error with stack trace
//     (log.Error) before re-throwing as a typed IntrospectionError (GEN-003).
//
// Every public function is wrapped by AppLogger.Logged.
//
// Since 1.4.0.
package schemaintrospector

import (
	"context"
	"database/sql"
	"strings"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
)

// DB is the minimal database surface the introspector needs. *database/sql.DB
// satisfies it; tests supply a fake. Since 1.4.0.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Column models one column of a table. Since 1.4.0.
type Column struct {
	Name     string
	DataType string
	Nullable bool
	Required bool // !Nullable
	Enum     []string
}

// Table models one table (or view) in the schema. Since 1.4.0.
type Table struct {
	Name    string
	IsView  bool
	Columns []Column
}

// Schema is the introspected schema. Since 1.4.0.
type Schema struct {
	Name   string
	Tables []Table
}

// Introspector reads the schema. Since 1.4.0.
type Introspector struct {
	log *applogger.Logger
	db  DB
}

// New constructs an Introspector bound to a DB. Since 1.4.0.
func New(log *applogger.Logger, db DB) *Introspector {
	if log == nil {
		log = applogger.New()
	}
	return &Introspector{log: log, db: db}
}

// Introspect reads all tables (and their columns) for the given schema name.
// Returns GEN-003 on any query failure. Since 1.4.0.
func (i *Introspector) Introspect(ctx context.Context, schemaName string) (*Schema, error) {
	return applogger.Logged1(i.log, "SchemaIntrospector.Introspect", func() (*Schema, error) {
		tables, err := i.queryTables(ctx, schemaName)
		if err != nil {
			return nil, err
		}
		for idx := range tables {
			cols, err := i.queryColumns(ctx, schemaName, tables[idx].Name)
			if err != nil {
				return nil, err
			}
			for cidx := range cols {
				if strings.EqualFold(cols[cidx].DataType, "USER-DEFINED") || isEnumType(cols[cidx].DataType) {
					labels, lerr := i.QueryEnumValues(ctx, schemaName, cols[cidx].DataType)
					if lerr != nil {
						// Non-fatal: keep the column without enum labels but log.
						i.log.Error("Introspect: enum lookup failed", "column", cols[cidx].Name, "err", lerr)
					} else {
						cols[cidx].Enum = labels
					}
				}
			}
			tables[idx].Columns = cols
		}
		return &Schema{Name: schemaName, Tables: tables}, nil
	})
}

// queryTables reads table+view names from information_schema. Since 1.4.0.
func (i *Introspector) queryTables(ctx context.Context, schemaName string) ([]Table, error) {
	rows, err := i.db.QueryContext(ctx, tablesSQL(), schemaName)
	if err != nil {
		i.log.Error("queryTables: query failed", "schema", schemaName, "err", err)
		return nil, errors.Wrap(errors.CodeIntrospection, "queryTables failed", err)
	}
	defer rows.Close()
	var out []Table
	for rows.Next() {
		var t Table
		var isView sql.NullBool
		if err := rows.Scan(&t.Name, &isView); err != nil {
			i.log.Error("queryTables: scan failed", "err", err)
			return nil, errors.Wrap(errors.CodeIntrospection, "queryTables scan failed", err)
		}
		t.IsView = isView.Bool
		out = append(out, t)
	}
	return out, rows.Err()
}

// queryColumns reads column metadata for one table. Since 1.4.0.
func (i *Introspector) queryColumns(ctx context.Context, schemaName, table string) ([]Column, error) {
	rows, err := i.db.QueryContext(ctx, columnsSQL(), schemaName, table)
	if err != nil {
		i.log.Error("queryColumns: query failed", "table", table, "err", err)
		return nil, errors.Wrap(errors.CodeIntrospection, "queryColumns failed", err)
	}
	defer rows.Close()
	var out []Column
	for rows.Next() {
		var c Column
		var nullable string
		var dataType string
		if err := rows.Scan(&c.Name, &dataType, &nullable); err != nil {
			i.log.Error("queryColumns: scan failed", "err", err)
			return nil, errors.Wrap(errors.CodeIntrospection, "queryColumns scan failed", err)
		}
		c.DataType = dataType
		c.Nullable = strings.EqualFold(nullable, "YES")
		c.Required = !c.Nullable
		out = append(out, c)
	}
	return out, rows.Err()
}

// QueryEnumValues returns the labels of a Postgres enum type.
//
// v1.4.0 (P1-3.9): the type name is passed via a parameter-bound query
// ($1 placeholder) — never string-interpolated. The regression test
// TestQueryEnumValues_UsesBoundParameter asserts the SQL text contains
// exactly one "$1" and zero concatenation. Since 1.4.0.
func (i *Introspector) QueryEnumValues(ctx context.Context, schemaName, typeName string) ([]string, error) {
	return applogger.Logged1(i.log, "SchemaIntrospector.QueryEnumValues", func() ([]string, error) {
		rows, err := i.db.QueryContext(ctx, enumValuesSQL(), typeName)
		if err != nil {
			// P1-3.4: log the cause with stack before rethrowing.
			i.log.Error("QueryEnumValues: query failed", "type", typeName, "err", err)
			return nil, errors.Wrap(errors.CodeIntrospection, "QueryEnumValues failed", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var label string
			if err := rows.Scan(&label); err != nil {
				i.log.Error("QueryEnumValues: scan failed", "err", err)
				return nil, errors.Wrap(errors.CodeIntrospection, "QueryEnumValues scan failed", err)
			}
			out = append(out, label)
		}
		return out, rows.Err()
	})
}

// IsView reports whether a relation in the given schema is a view rather than
// a base table. P1-3.4: logs the cause with stack before rethrowing.
// Since 1.4.0.
func (i *Introspector) IsView(ctx context.Context, schemaName, tableName string) (bool, error) {
	return applogger.Logged1(i.log, "SchemaIntrospector.IsView", func() (bool, error) {
		var isView sql.NullBool
		err := i.db.QueryRowContext(ctx, isViewSQL(), schemaName, tableName).Scan(&isView)
		if err != nil {
			i.log.Error("IsView: query failed", "table", tableName, "err", err)
			return false, errors.Wrap(errors.CodeIntrospection, "IsView failed", err)
		}
		return isView.Bool, nil
	})
}

// isEnumType heuristically detects Postgres enum types by name.
// Since 1.4.0.
func isEnumType(dt string) bool {
	return strings.EqualFold(dt, "enum") || strings.HasSuffix(strings.ToLower(dt), "_enum")
}

// ---- SQL constants (kept here so tests can inspect them for the
// parameter-binding regression). ---------------------------------------------

// tablesSQL lists base tables and views for a schema. Since 1.4.0.
func tablesSQL() string {
	return `SELECT table_name, table_type = 'VIEW' AS is_view
	        FROM information_schema.tables
	        WHERE table_schema = $1
	        ORDER BY table_name`
}

// columnsSQL lists columns for a table. Since 1.4.0.
func columnsSQL() string {
	return `SELECT column_name, data_type, is_nullable
	        FROM information_schema.columns
	        WHERE table_schema = $1 AND table_name = $2
	        ORDER BY ordinal_position`
}

// enumValuesSQL returns the labels of a Postgres enum type.
//
// v1.4.0 / P1-3.9: $1 is a bound parameter — the type name is NEVER
// interpolated. Since 1.4.0.
func enumValuesSQL() string {
	return `SELECT e.enumlabel
	        FROM pg_type t
	        JOIN pg_enum e ON e.enumtypid = t.oid
	        WHERE t.typname = $1
	        ORDER BY e.enumsortorder`
}

// isViewSQL looks up the table_type for one relation. Since 1.4.0.
func isViewSQL() string {
	return `SELECT table_type = 'VIEW'
	        FROM information_schema.tables
	        WHERE table_schema = $1 AND table_name = $2`
}
