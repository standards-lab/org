package query

import (
	"fmt"
	"regexp"
	"strings"
)

// A parameter is written {{name}}, or {{name:type}} to bind it through
// CAST to an SQL type — standard or the engine's own, as the file's tier
// declares; the type is the author's and reaches the engine verbatim, where
// Verify catches a name the engine does not know. Whitespace inside the
// braces is allowed. The delimiter is reserved: it means a parameter
// wherever it appears in the body, string literals and comments included,
// so the body needs no lexer, and a {{ that does not form a parameter is a
// load error.
var (
	parameter = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*(` + typeToken + `)\s*)?\}\}`)
	opener    = "{{"
)

// typeToken is an SQL type as written in a cast or a field contract: a
// name, optionally with a length or precision, such as text, uuid,
// numeric(12,2), or varchar(200).
const typeToken = `[A-Za-z_][A-Za-z0-9_ ]*?(?:\([0-9]+(?:\s*,\s*[0-9]+)?\))?`

var sqlType = regexp.MustCompile(`^` + typeToken + `$`)

// rewrite resolves the parameters of body to the dialect's positional
// placeholders and returns the names in position order. Each distinct name
// takes the position of its first occurrence; later occurrences rebind to
// it. An occurrence with a type renders as CAST(<placeholder> AS <type>).
func rewrite(body string, placeholder func(int) string) (string, []string, error) {
	var out strings.Builder
	var names []string
	position := map[string]int{}
	last := 0
	for _, m := range parameter.FindAllStringSubmatchIndex(body, -1) {
		if stray := strings.Index(body[last:m[0]], opener); stray >= 0 {
			return "", nil, malformed(body, last+stray)
		}
		out.WriteString(body[last:m[0]])
		name := body[m[2]:m[3]]
		p, ok := position[name]
		if !ok {
			names = append(names, name)
			p = len(names)
			position[name] = p
		}
		if m[4] >= 0 {
			out.WriteString("CAST(")
			out.WriteString(placeholder(p))
			out.WriteString(" AS ")
			out.WriteString(strings.TrimSpace(body[m[4]:m[5]]))
			out.WriteString(")")
		} else {
			out.WriteString(placeholder(p))
		}
		last = m[1]
	}
	if stray := strings.Index(body[last:], opener); stray >= 0 {
		return "", nil, malformed(body, last+stray)
	}
	out.WriteString(body[last:])
	return out.String(), names, nil
}

// malformed reports a {{ that does not open a parameter, quoting the text
// from it to the end of its line.
func malformed(body string, at int) error {
	rest := body[at:]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	return fmt.Errorf("malformed parameter %q: want {{name}} or {{name:type}}", rest)
}
