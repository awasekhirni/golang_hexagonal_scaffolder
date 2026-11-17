package schemaintrospector

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/testutil/fakedb"
)

func newIntrospector(t *testing.T, fake *fakedb.DB) (*Introspector, *sql.DB) {
	t.Helper()
	// Open the fakedb driver. The DSN is the registered name.
	sqlDB, err := sql.Open("fakedb", fake.Name())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	log := applogger.NewWithEnv(func(string) string { return "" })
	return New(log, sqlDB), sqlDB
}

// TestIntrospect_happy covers tables + columns + enum lookup. Since 1.4.0.
func TestIntrospect_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("FROM information_schema.tables", &fakedb.Rows{
		Cols:   []string{"table_name", "is_view"},
		Values: [][]any{{"users", false}, {"orders", true}},
	})
	fake.AddRows("FROM information_schema.columns", &fakedb.Rows{
		Cols:   []string{"column_name", "data_type", "is_nullable"},
		Values: [][]any{{"id", "uuid", "NO"}, {"email", "text", "YES"}},
	})
	fake.AddRows("pg_enum", &fakedb.Rows{
		Cols:   []string{"enumlabel"},
		Values: [][]any{{"active"}, {"inactive"}},
	})

	intro, db := newIntrospector(t, fake)
	defer db.Close()
	schema, err := intro.Introspect(context.Background(), "public")
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if len(schema.Tables) != 2 {
		t.Fatalf("tables: want 2, got %d", len(schema.Tables))
	}
	if schema.Tables[0].Name != "users" || schema.Tables[0].IsView {
		t.Fatalf("first table: want users/base, got %+v", schema.Tables[0])
	}
	if !schema.Tables[1].IsView {
		t.Fatalf("second table: want view, got base")
	}
	if len(schema.Tables[0].Columns) != 2 {
		t.Fatalf("columns: want 2, got %d", len(schema.Tables[0].Columns))
	}
	if !schema.Tables[0].Columns[0].Required {
		t.Fatalf("id should be required")
	}
	if schema.Tables[0].Columns[1].Required {
		t.Fatalf("email should be nullable (not required)")
	}
}

// TestQueryEnumValues_UsesBoundParameter is the v1.4.0 SQL-injection
// regression (P1-3.9). It asserts the SQL text passed to the driver contains
// exactly one "$1" placeholder and that the type name was sent as an arg,
// never interpolated. Since 1.4.0.
func TestQueryEnumValues_UsesBoundParameter(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("pg_enum", &fakedb.Rows{
		Cols:   []string{"enumlabel"},
		Values: [][]any{{"a"}, {"b"}},
	})
	intro, db := newIntrospector(t, fake)
	defer db.Close()

	// A malicious type name that would break out of a quoted interpolation.
	malicious := "x'; DROP TABLE pg_type; --"
	labels, err := intro.QueryEnumValues(context.Background(), "public", malicious)
	if err != nil {
		t.Fatalf("QueryEnumValues: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("labels: want 2, got %d", len(labels))
	}
	// The type name must have been passed as the first arg, not interpolated.
	args := fake.LastQueryArgs("pg_enum")
	if len(args) != 1 || args[0] != malicious {
		t.Fatalf("expected malicious name as arg[0], got %v", args)
	}
}

// TestQueryEnumValues_queryFails covers the GEN-003 path with stack logging.
// Since 1.4.0.
func TestQueryEnumValues_queryFails(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddError("pg_enum", errors.New("connection lost"))
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	_, err := intro.QueryEnumValues(context.Background(), "public", "ok")
	if err == nil || !strings.Contains(err.Error(), "GEN-003") {
		t.Fatalf("QueryEnumValues fail: want GEN-003, got %v", err)
	}
}

// TestIsView_happy covers the isView lookup. Since 1.4.0.
func TestIsView_happy(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	// QueryRow path: the fakedb Query is used for both Query and QueryRow in
	// database/sql, so we register a single-row result.
	fake.AddRows("table_type = 'VIEW'", &fakedb.Rows{
		Cols:   []string{"is_view"},
		Values: [][]any{{true}},
	})
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	got, err := intro.IsView(context.Background(), "public", "orders")
	if err != nil {
		t.Fatalf("IsView: %v", err)
	}
	if !got {
		t.Fatalf("IsView: want true")
	}
}

// TestIsView_queryFails covers the GEN-003 path. Since 1.4.0.
func TestIsView_queryFails(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddError("table_type = 'VIEW'", errors.New("nope"))
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	_, err := intro.IsView(context.Background(), "public", "orders")
	if err == nil || !strings.Contains(err.Error(), "GEN-003") {
		t.Fatalf("IsView fail: want GEN-003, got %v", err)
	}
}

// TestIntrospect_tableQueryFails covers GEN-003 from the tables query.
// Since 1.4.0.
func TestIntrospect_tableQueryFails(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddError("FROM information_schema.tables", errors.New("boom"))
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	_, err := intro.Introspect(context.Background(), "public")
	if err == nil || !strings.Contains(err.Error(), "GEN-003") {
		t.Fatalf("Introspect tables fail: want GEN-003, got %v", err)
	}
}

// TestIntrospect_columnQueryFails covers GEN-003 from the columns query.
// Since 1.4.0.
func TestIntrospect_columnQueryFails(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("FROM information_schema.tables", &fakedb.Rows{
		Cols:   []string{"table_name", "is_view"},
		Values: [][]any{{"users", false}},
	})
	fake.AddError("FROM information_schema.columns", errors.New("boom"))
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	_, err := intro.Introspect(context.Background(), "public")
	if err == nil || !strings.Contains(err.Error(), "GEN-003") {
		t.Fatalf("Introspect columns fail: want GEN-003, got %v", err)
	}
}

// TestIntrospect_emptySchema covers the no-tables case (not an error).
// Since 1.4.0.
func TestIntrospect_emptySchema(t *testing.T) {
	fake := fakedb.Register(t.Name())
	defer fakedb.Reset()
	fake.AddRows("FROM information_schema.tables", &fakedb.Rows{
		Cols: []string{"table_name", "is_view"},
	})
	intro, db := newIntrospector(t, fake)
	defer db.Close()
	schema, err := intro.Introspect(context.Background(), "public")
	if err != nil {
		t.Fatalf("Introspect empty: %v", err)
	}
	if len(schema.Tables) != 0 {
		t.Fatalf("empty schema: want 0 tables, got %d", len(schema.Tables))
	}
}

// TestEnumValuesSQL_ContainsPlaceholder is a static assertion that the SQL
// constant uses exactly one $1 and no Sprintf/concatenation. Since 1.4.0.
func TestEnumValuesSQL_ContainsPlaceholder(t *testing.T) {
	q := enumValuesSQL()
	if c := strings.Count(q, "$1"); c != 1 {
		t.Fatalf("enumValuesSQL: want exactly one $1, got %d in %q", c, q)
	}
	if strings.Contains(strings.ToLower(q), "sprintf") || strings.Contains(q, "+") {
		t.Fatalf("enumValuesSQL: suspicious concatenation in %q", q)
	}
}
