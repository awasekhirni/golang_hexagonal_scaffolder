// Package templateengine wraps text/template with the helpers and the
// parse/execute API used by the generator.
//
// Helpers registered: Pascal, Camel, Snake, RouteName, Lower, Upper, TypeMap,
// Comment, Since. A missing helper or an execution error returns a typed
// TemplateError (GEN-007 / GEN-004).
//
// Every public function is wrapped by AppLogger.Logged.
//
// Since 1.4.0.
package templateengine

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
	"github.com/example/hexapi/generator/internal/naming"
)

// Engine is the template execution engine. Since 1.4.0.
type Engine struct {
	log     *applogger.Logger
	naming  *naming.Naming
	parsed  map[string]*template.Template
	typeMap map[string]string
}

// New constructs an Engine bound to a Naming service. Since 1.4.0.
func New(log *applogger.Logger, n *naming.Naming) *Engine {
	if log == nil {
		log = applogger.New()
	}
	if n == nil {
		n = naming.New(log)
	}
	return &Engine{
		log:     log,
		naming:  n,
		parsed:  map[string]*template.Template{},
		typeMap: defaultTypeMap(),
	}
}

// defaultTypeMap is the SQL-type → Go-type mapping. Since 1.4.0.
func defaultTypeMap() map[string]string {
	return map[string]string{
		"int":        "int",
		"integer":    "int",
		"int4":       "int",
		"int8":       "int64",
		"bigint":     "int64",
		"smallint":   "int16",
		"int2":       "int16",
		"serial":     "int",
		"bigserial":  "int64",
		"text":       "string",
		"varchar":    "string",
		"char":       "string",
		"uuid":       "string",
		"boolean":    "bool",
		"bool":       "bool",
		"timestamp":  "time.Time",
		"timestamptz": "time.Time",
		"date":       "time.Time",
		"numeric":    "float64",
		"decimal":    "float64",
		"float8":     "float64",
		"float4":     "float32",
		"real":       "float32",
		"jsonb":      "json.RawMessage",
		"json":       "json.RawMessage",
	}
}

// Parse parses one or more named templates from their source text. The name
// is used later by Execute. Re-parsing the same name replaces it.
// Returns GEN-004 on a parse error. Since 1.4.0.
func (e *Engine) Parse(name, src string) error {
	return e.log.Logged("TemplateEngine.Parse", func() error {
		t, err := template.New(name).Funcs(e.funcMap()).Parse(src)
		if err != nil {
			e.log.Error("TemplateEngine.Parse: parse failed", "name", name, "err", err)
			return errors.Wrap(errors.CodeTemplate, "parse failed: "+name, err)
		}
		e.parsed[name] = t
		return nil
	})
}

// Execute renders the named template against data. Returns GEN-004 on an
// execution error. Since 1.4.0.
func (e *Engine) Execute(name string, data any) (string, error) {
	return applogger.Logged1(e.log, "TemplateEngine.Execute", func() (string, error) {
		t, ok := e.parsed[name]
		if !ok {
			e.log.Error("TemplateEngine.Execute: template not parsed", "name", name)
			return "", errors.New(errors.CodeTemplate, "template not parsed: "+name)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			e.log.Error("TemplateEngine.Execute: exec failed", "name", name, "err", err)
			return "", errors.Wrap(errors.CodeGeneration, "execute failed: "+name, err)
		}
		return buf.String(), nil
	})
}

// ExecuteAll renders every parsed template keyed by name into a map of
// path→content. The path comes from the data's Path() if it implements
// pathProvider; otherwise the template name is used. Since 1.4.0.
func (e *Engine) ExecuteAll(data any) (map[string]string, error) {
	return applogger.Logged1(e.log, "TemplateEngine.ExecuteAll", func() (map[string]string, error) {
		out := make(map[string]string, len(e.parsed))
		for name := range e.parsed {
			rendered, err := e.Execute(name, data)
			if err != nil {
				return nil, err
			}
			out[name] = rendered
		}
		return out, nil
	})
}

// Names returns the names of all parsed templates. Since 1.4.0.
func (e *Engine) Names() []string {
	var names []string
	for n := range e.parsed {
		names = append(names, n)
	}
	return names
}

// funcMap returns the template helper map. Since 1.4.0.
func (e *Engine) funcMap() template.FuncMap {
	return template.FuncMap{
		"Pascal":   e.wrap(e.naming.Pascal),
		"Camel":    e.wrap(e.naming.Camel),
		"Snake":    e.wrap(e.naming.Snake),
		"Route":    e.wrap(e.naming.RouteName),
		"Entity":   e.wrap(e.naming.EntityName),
		"GoName":   e.wrap(e.naming.GoName),
		"Lower":    strings.ToLower,
		"Upper":    strings.ToUpper,
		"TypeMap":  e.goType,
		"Comment":  commentLines,
		"Since":    sinceTag,
	}
}

// wrap converts the (string, error) Naming API into the single-string
// template-helper API: an error panics during execution, which text/template
// catches and surfaces as an exec error. Since 1.4.0.
func (e *Engine) wrap(fn func(string) (string, error)) func(string) string {
	return func(s string) string {
		v, err := fn(s)
		if err != nil {
			panic(fmt.Sprintf("naming helper failed for %q: %v", s, err))
		}
		return v
	}
}

// goType maps an SQL type name to its Go type. Since 1.4.0.
func (e *Engine) goType(sqlType string) string {
	if t, ok := e.typeMap[strings.ToLower(strings.TrimSpace(sqlType))]; ok {
		return t
	}
	// Unknown types default to string; the generator logs at debug so the
	// user can extend the map if they care.
	e.log.Debug("TypeMap: unknown SQL type, defaulting to string", "sql_type", sqlType)
	return "string"
}

// commentLines prefixes each line of s with "// ". Since 1.4.0.
func commentLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "// " + l
	}
	return strings.Join(lines, "\n")
}

// sinceTag renders the standard doc comment. Since 1.4.0.
func sinceTag(v string) string {
	if v == "" {
		return "// Since 1.4.0."
	}
	return "// Since " + v + "."
}
