package volume_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
)

// TestParseAddress fixes the command line's address form: <volume>:<path>
// with the path starting at /, the volume's root as docs:/, and a refusal
// naming the reason for a missing colon, an empty volume, and a path that
// does not start with /.
func TestParseAddress(t *testing.T) {
	for in, want := range map[string]volume.Address{
		"docs:/":              {Volume: "docs", Path: "/"},
		"docs:/reports/2026":  {Volume: "docs", Path: "/reports/2026"},
		"my docs:/a b":        {Volume: "my docs", Path: "/a b"},
		"docs:/with:colon/x":  {Volume: "docs", Path: "/with:colon/x"},
		"docs:/trailing/":     {Volume: "docs", Path: "/trailing/"},
		"café:/reports/2026/": {Volume: "café", Path: "/reports/2026/"},
	} {
		got, err := volume.ParseAddress(in)
		if err != nil || got != want {
			t.Errorf("ParseAddress(%q) = %+v, %v, want %+v", in, got, err, want)
		}
		if got.String() != in {
			t.Errorf("ParseAddress(%q).String() = %q", in, got.String())
		}
	}
	for in, reason := range map[string]string{
		"docs":        "no colon",
		"/reports":    "no colon",
		":/reports":   "names no volume",
		"docs:":       "does not start with /",
		"docs:a/b":    "does not start with /",
		"docs:./a":    "does not start with /",
		"docs:\\a\\b": "does not start with /",
	} {
		_, err := volume.ParseAddress(in)
		if !errors.Is(err, volume.ErrInvalidAddress) {
			t.Errorf("ParseAddress(%q) = %v, want ErrInvalidAddress", in, err)
			continue
		}
		if !strings.Contains(err.Error(), reason) {
			t.Errorf("ParseAddress(%q) = %q, want the reason %q", in, err, reason)
		}
	}
}

// TestAddressSplit fixes how mkdir reads an address: the parent's address
// and the last segment, the parent of a first-level directory being the
// root; the root itself and a path ending with a slash are refused.
func TestAddressSplit(t *testing.T) {
	for in, want := range map[string]struct {
		parent, name string
	}{
		"docs:/a":     {"docs:/", "a"},
		"docs:/a/b":   {"docs:/a", "b"},
		"docs:/a/b/c": {"docs:/a/b", "c"},
		"docs:/a//b":  {"docs:/a/", "b"},
	} {
		addr, err := volume.ParseAddress(in)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", in, err)
		}
		parent, name, err := addr.Split()
		if err != nil || parent.String() != want.parent || name != want.name {
			t.Errorf("%s.Split() = %s, %q, %v; want %s, %q", in, parent, name, err, want.parent, want.name)
		}
		if parent.Join(name) != addr {
			t.Errorf("%s.Join(%q) = %s, want %s", parent, name, parent.Join(name), addr)
		}
	}
	for _, in := range []string{"docs:/", "docs:/a/", "docs:/a/b/"} {
		addr, err := volume.ParseAddress(in)
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", in, err)
		}
		if _, _, err := addr.Split(); !errors.Is(err, volume.ErrInvalidAddress) {
			t.Errorf("%s.Split() = %v, want ErrInvalidAddress", in, err)
		}
	}
}
