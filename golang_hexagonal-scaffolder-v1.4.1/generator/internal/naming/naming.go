// Package naming converts database identifiers (snake_case, kebab-case,
// SCREAMING_SNAKE) into Go-safe identifiers in the conventions used by the
// generated hexagonal API: PascalCase for types, camelCase for fields,
// snake_case preserved for JSON tags and SQL column lists.
//
// Every public function is wrapped by AppLogger.Logged. If an identifier
// cannot be sanitised to a valid Go identifier (e.g. it starts with a digit
// after stripping), the function returns a typed NamingError (GEN-005).
//
// Since 1.4.0.
package naming

import (
	"strings"
	"unicode"

	"github.com/example/hexapi/generator/internal/applogger"
	"github.com/example/hexapi/generator/internal/errors"
)

// acronyms maps lowercased words to their conventional Go all-caps form so
// that "user_id" → "UserID" (not "UserId") and "id" → "ID". Since 1.4.0.
var acronyms = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "uuid": "UUID",
	"http": "HTTP", "https": "HTTPS", "ssl": "SSL", "tls": "TLS",
	"sql": "SQL", "db": "DB", "api": "API", "json": "JSON", "xml": "XML",
	"html": "HTML", "css": "CSS", "js": "JS", "ip": "IP", "tcp": "TCP",
	"udp": "UDP", "cpu": "CPU", "gpu": "GPU", "ram": "RAM", "utc": "UTC",
	"crc": "CRC", "sha": "SHA", "hmac": "HMAC", "jwt": "JWT", "oauth": "OAuth",
	"smtp": "SMTP", "ftp": "FTP", "ssh": "SSH", "dns": "DNS", "cdn": "CDN",
	"crud": "CRUD", "orm": "ORM", "dto": "DTO", "dao": "DAO",
}

// Naming is the case-conversion service. Construct once, reuse.
// Since 1.4.0.
type Naming struct {
	log *applogger.Logger
}

// New constructs a Naming. Since 1.4.0.
func New(log *applogger.Logger) *Naming {
	if log == nil {
		log = applogger.New()
	}
	return &Naming{log: log}
}

// Pascal converts snake/kebab/space-separated input to PascalCase.
// e.g. "user_account-id" → "UserAccountId". Since 1.4.0.
func (n *Naming) Pascal(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.Pascal", func() (string, error) {
		return n.toPascal(in)
	})
}

// Camel converts to camelCase (first word lowercase). Since 1.4.0.
func (n *Naming) Camel(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.Camel", func() (string, error) {
		p, err := n.toPascal(in)
		if err != nil {
			return "", err
		}
		if p == "" {
			return "", nil
		}
		r := []rune(p)
		r[0] = unicode.ToLower(r[0])
		return string(r), nil
	})
}

// Snake converts to lower snake_case. Since 1.4.0.
func (n *Naming) Snake(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.Snake", func() (string, error) {
		return toSnake(in), nil
	})
}

// RouteName returns the plural, kebab-case URL segment for a table.
// e.g. "user_account" → "user-accounts". Since 1.4.0.
func (n *Naming) RouteName(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.RouteName", func() (string, error) {
		snake := toSnake(in)
		if snake == "" {
			return "", nil
		}
		if !endsWith(snake, 's') {
			snake += "s"
		}
		return strings.ReplaceAll(snake, "_", "-"), nil
	})
}

// EntityName returns the PascalCase singular entity name for a table.
// e.g. "user_accounts" → "UserAccount". Since 1.4.0.
func (n *Naming) EntityName(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.EntityName", func() (string, error) {
		singular := singularize(toSnake(in))
		return n.toPascal(singular)
	})
}

// Sanitise produces a Go-identifier-safe form of an arbitrary string: keeps
// [A-Za-z0-9_], lowercases, prefixes "_" if it starts with a digit. If the
// result is empty, returns GEN-005. Since 1.4.0.
func (n *Naming) Sanitise(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.Sanitise", func() (string, error) {
		var b strings.Builder
		for _, r := range in {
			switch {
			case r >= 'A' && r <= 'Z':
				b.WriteRune(unicode.ToLower(r))
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == '_' || r == '-':
				b.WriteRune('_')
			}
		}
		out := b.String()
		if out == "" {
			n.log.Error("Naming.Sanitise: empty after sanitising", "input", in)
			return "", errors.New(errors.CodeNaming, "identifier sanitises to empty: "+in)
		}
		if out[0] >= '0' && out[0] <= '9' {
			out = "_" + out
		}
		return out, nil
	})
}

// GoName returns the exported Go field name for a column: PascalCase of the
// column name. Since 1.4.0.
func (n *Naming) GoName(in string) (string, error) {
	return applogger.Logged1(n.log, "Naming.GoName", func() (string, error) {
		return n.toPascal(in)
	})
}

// toPascal is the unwrapped worker. Since 1.4.0.
func (n *Naming) toPascal(in string) (string, error) {
	if in == "" {
		return "", nil
	}
	words := splitWords(in)
	if len(words) == 0 {
		n.log.Error("Naming.toPascal: no words after split", "input", in)
		return "", errors.New(errors.CodeNaming, "identifier has no usable words: "+in)
	}
	var b strings.Builder
	for _, w := range words {
		if w == "" {
			continue
		}
		// Acronyms keep their conventional all-caps form (id → ID, url → URL).
		if ac, ok := acronyms[strings.ToLower(w)]; ok {
			b.WriteString(ac)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	out := b.String()
	if out == "" {
		return "", errors.New(errors.CodeNaming, "identifier sanitises to empty: "+in)
	}
	// Ensure it does not start with a digit.
	if out[0] >= '0' && out[0] <= '9' {
		return "X" + out, nil
	}
	return out, nil
}

// splitWords breaks an identifier on '_', '-', ' ', and case boundaries.
// Since 1.4.0.
func splitWords(in string) []string {
	in = strings.TrimSpace(in)
	if in == "" {
		return nil
	}
	// Normalise separators to underscore.
	for _, sep := range []string{"-", " "} {
		in = strings.ReplaceAll(in, sep, "_")
	}
	parts := strings.Split(in, "_")
	var words []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		// Split camelCase boundaries inside a part: "userId" → "user" "Id".
		words = append(words, splitCamel(p)...)
	}
	return words
}

// splitCamel splits "userId" → ["user", "Id"], "UserID" → ["User", "ID"].
// Since 1.4.0.
func splitCamel(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	var out []string
	var b strings.Builder
	for i, r := range runes {
		if i == 0 {
			b.WriteRune(r)
			continue
		}
		prev := runes[i-1]
		// Boundary: lowercase→uppercase ("userId" at the 'I').
		if unicode.IsLower(prev) && unicode.IsUpper(r) {
			out = append(out, b.String())
			b.Reset()
		}
		// Boundary: uppercase→uppercase→lowercase ("UserID" at the 'I' before 'd').
		if i+1 < len(runes) && unicode.IsUpper(prev) && unicode.IsUpper(r) && unicode.IsLower(runes[i+1]) {
			out = append(out, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// toSnake lowercases and joins on underscore. Since 1.4.0.
func toSnake(in string) string {
	words := splitWords(in)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

// endsWith reports whether s ends with the byte c. Since 1.4.0.
func endsWith(s string, c rune) bool {
	if s == "" {
		return false
	}
	r := []rune(s)
	return r[len(r)-1] == c
}

// singularize applies a few English rules. Not exhaustive; good enough for
// typical table names. Since 1.4.0.
func singularize(snake string) string {
	if snake == "" {
		return snake
	}
	switch {
	case strings.HasSuffix(snake, "ies") && len(snake) > 3:
		return snake[:len(snake)-3] + "y"
	case strings.HasSuffix(snake, "ses"):
		return snake[:len(snake)-2]
	case strings.HasSuffix(snake, "s") && !strings.HasSuffix(snake, "ss"):
		return snake[:len(snake)-1]
	}
	return snake
}
