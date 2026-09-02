package sqlheader

import (
	"bufio"
	"regexp"
	"strings"
)

// Directive is one "-- key: value" line of the header, with its 1-based
// line number for a consumer's error messages.
type Directive struct {
	Key   string
	Value string
	Line  int
}

// Header is the directives of one file, in file order. A key may repeat;
// "field" does, once per column.
type Header struct {
	directives []Directive
}

var directive = regexp.MustCompile(`^([a-z][a-z0-9_-]*):\s*(.*)$`)

// Parse reads the header from text. It cannot fail: a line that is not a
// directive is prose, and the header ends at the first non-comment line.
func Parse(text string) Header {
	var h Header
	sc := bufio.NewScanner(strings.NewReader(text))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "--") {
			break
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "--"))
		if m := directive.FindStringSubmatch(body); m != nil {
			h.directives = append(h.directives, Directive{Key: m[1], Value: strings.TrimSpace(m[2]), Line: n})
		}
	}
	return h
}

// Directives returns every directive in file order.
func (h Header) Directives() []Directive {
	out := make([]Directive, len(h.directives))
	copy(out, h.directives)
	return out
}

// Get returns the value of the first directive with key, and whether one
// exists.
func (h Header) Get(key string) (string, bool) {
	for _, d := range h.directives {
		if d.Key == key {
			return d.Value, true
		}
	}
	return "", false
}

// All returns the values of every directive with key, in order.
func (h Header) All(key string) []string {
	var out []string
	for _, d := range h.directives {
		if d.Key == key {
			out = append(out, d.Value)
		}
	}
	return out
}

// Keys returns the distinct keys in order of first appearance, so a consumer
// can reject the ones it does not know.
func (h Header) Keys() []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range h.directives {
		if !seen[d.Key] {
			seen[d.Key] = true
			out = append(out, d.Key)
		}
	}
	return out
}
