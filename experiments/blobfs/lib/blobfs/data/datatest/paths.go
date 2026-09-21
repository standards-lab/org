package datatest

import (
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// hostileNames are directory names that would break a path spliced into
// SQL text or into a Postgres array literal, and that ValidateName
// accepts: quotes, a backslash, braces, a comma, an SQL comment and
// statement terminator, a NUL-free control-free unicode name, and
// leading and trailing spaces.
var hostileNames = []string{
	`a"b`,
	`a\b`,
	`{x}`,
	`a,b`,
	`'; DROP TABLE blobfs_directory; --`,
	`caf` + string(rune(0x00E9)) + ` ` + string(rune(0x4E2D)) + string(rune(0x6587)),
	` leading`,
	`trailing `,
	`NULL`,
	`"quoted"`,
	`back\\slash\n`,
}

// resolvePath checks path resolution through the variant against the
// baseline: the same directory for every depth, the same error text for
// a missing segment at every position and for a missing start, and the
// same outcome for names that would break a spliced path.
func (s *suite) resolvePath(t *testing.T) {
	top := s.mkdir(t, "resolve-"+t.Name())
	// A chain of depth six below top: top/d1/d2/d3/d4/d5/d6.
	chain := []blobfs.Directory{top}
	names := []string{top.Name}
	for i := 1; i <= 6; i++ {
		name := "d" + string(rune('0'+i))
		chain = append(chain, s.mkdirUnder(t, chain[i-1].ID, name))
		names = append(names, name)
	}
	abs := func(n int) string { return "/" + strings.Join(names[:n], "/") }
	rel := func(from, n int) string { return strings.Join(names[from:n], "/") }

	t.Run("Depths", func(t *testing.T) {
		for _, depth := range []int{0, 1, 3, 6} {
			n := depth + 1 // names[:n] is the path to chain[depth].
			if depth == 0 {
				n = 0
			}
			want := chain[depth]
			if depth == 0 {
				want = s.directory(t, blobfs.RootID)
			}
			got, err := s.store.ResolveDirectory(s.ctx, s.db, abs(n))
			if err != nil || !equalDirectory(got, want) {
				t.Errorf("ResolveDirectory(%q) = %+v, %v, want %+v", abs(n), got, err, want)
			}
			base, err := s.standard.ResolveDirectory(s.ctx, s.db, abs(n))
			if err != nil || !equalDirectory(base, got) {
				t.Errorf("the baseline's ResolveDirectory(%q) = %+v, %v, want the variant's %+v", abs(n), base, err, got)
			}
			// The relative form from top, which reaches chain[depth] with
			// depth segments; from top itself the path is empty.
			if depth == 0 {
				continue
			}
			got, err = s.store.ResolveDirectoryFrom(s.ctx, s.db, top.ID, rel(1, n))
			if err != nil || !equalDirectory(got, chain[depth]) {
				t.Errorf("ResolveDirectoryFrom(top, %q) = %+v, %v, want %+v", rel(1, n), got, err, chain[depth])
			}
			base, err = s.standard.ResolveDirectoryFrom(s.ctx, s.db, top.ID, rel(1, n))
			if err != nil || !equalDirectory(base, got) {
				t.Errorf("the baseline's ResolveDirectoryFrom(top, %q) = %+v, %v, want the variant's", rel(1, n), base, err)
			}
		}
		for _, empty := range []string{"", "."} {
			got, err := s.store.ResolveDirectoryFrom(s.ctx, s.db, chain[3].ID, empty)
			if err != nil || !equalDirectory(got, chain[3]) {
				t.Errorf("ResolveDirectoryFrom(d3, %q) = %+v, %v, want d3 itself", empty, got, err)
			}
		}
	})
	t.Run("MissingSegment", func(t *testing.T) {
		for _, c := range []struct {
			name string
			path string
			at   string
		}{
			{"First", "/missing/" + rel(1, 4), "/missing"},
			{"Middle", abs(3) + "/missing/" + rel(3, 7), abs(3) + "/missing"},
			{"Last", abs(6) + "/missing", abs(6) + "/missing"},
		} {
			t.Run(c.name, func(t *testing.T) {
				_, err := s.store.ResolveDirectory(s.ctx, s.db, c.path)
				if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), " at "+c.at+": ") {
					t.Errorf("ResolveDirectory(%q) = %v, want ErrNotFound at %s", c.path, err, c.at)
				}
				_, base := s.standard.ResolveDirectory(s.ctx, s.db, c.path)
				s.wantSameError(t, err, base)
				// The same path relative to the root, and relative to top.
				relPath := strings.TrimPrefix(c.path, "/")
				_, err = s.store.ResolveDirectoryFrom(s.ctx, s.db, blobfs.RootID, relPath)
				if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), " at "+strings.TrimPrefix(c.at, "/")+": ") {
					t.Errorf("ResolveDirectoryFrom(root, %q) = %v, want ErrNotFound at %s", relPath, err, strings.TrimPrefix(c.at, "/"))
				}
				_, base = s.standard.ResolveDirectoryFrom(s.ctx, s.db, blobfs.RootID, relPath)
				s.wantSameError(t, err, base)
			})
		}
		below := rel(1, 4) + "/missing/x"
		_, err := s.store.ResolveDirectoryFrom(s.ctx, s.db, top.ID, below)
		if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), " at "+rel(1, 4)+"/missing: ") {
			t.Errorf("ResolveDirectoryFrom(top, %q) = %v, want ErrNotFound at the missing segment", below, err)
		}
		_, base := s.standard.ResolveDirectoryFrom(s.ctx, s.db, top.ID, below)
		s.wantSameError(t, err, base)
	})
	t.Run("MissingStart", func(t *testing.T) {
		file := s.insertFile(t, top.ID, "not-a-directory.txt", blobfs.StatusAvailable)
		for _, start := range []string{blobfs.NewID(), file} {
			for _, path := range []string{"", names[1]} {
				_, err := s.store.ResolveDirectoryFrom(s.ctx, s.db, start, path)
				if !errors.Is(err, blobfs.ErrNotFound) || strings.Contains(err.Error(), " at ") {
					t.Errorf("ResolveDirectoryFrom(%s, %q) = %v, want ErrNotFound for the start and no failing prefix", start, path, err)
				}
				_, base := s.standard.ResolveDirectoryFrom(s.ctx, s.db, start, path)
				s.wantSameError(t, err, base)
			}
		}
	})
	t.Run("HostileNames", func(t *testing.T) {
		hostile := s.mkdir(t, "hostile-"+t.Name())
		for _, name := range hostileNames {
			// Created through the store directly: the suite's own naming
			// helper rewrites slashes and backslashes for subtest names.
			d, err := s.store.Mkdir(s.ctx, s.db, hostile.ID, name)
			if err != nil {
				t.Fatalf("Mkdir(%q): %v", name, err)
			}
			child := s.mkdirUnder(t, d.ID, "child")
			path := "/" + hostile.Name + "/" + name + "/child"
			got, err := s.store.ResolveDirectory(s.ctx, s.db, path)
			if err != nil || !equalDirectory(got, child) {
				t.Errorf("ResolveDirectory(%q) = %+v, %v, want the child under the hostile name", path, got, err)
			}
			base, err := s.standard.ResolveDirectory(s.ctx, s.db, path)
			if err != nil || !equalDirectory(base, got) {
				t.Errorf("the baseline's ResolveDirectory(%q) = %+v, %v, want the variant's", path, base, err)
			}
			got, err = s.store.ResolveDirectoryFrom(s.ctx, s.db, hostile.ID, name+"/child")
			if err != nil || !equalDirectory(got, child) {
				t.Errorf("ResolveDirectoryFrom(hostile, %q) = %+v, %v, want the child", name+"/child", got, err)
			}
			// The name that does not exist under the same parent is not
			// found, so no character of it reached the engine as syntax.
			_, err = s.store.ResolveDirectory(s.ctx, s.db, "/"+hostile.Name+"/"+name+"/absent")
			if !errors.Is(err, blobfs.ErrNotFound) {
				t.Errorf("ResolveDirectory below %q of an absent name = %v, want ErrNotFound", name, err)
			}
		}
		// Each of a, b, and a,b is its own directory: the comma is a
		// character of a name, not a separator.
		a := s.mkdirUnder(t, hostile.ID, "a")
		b := s.mkdirUnder(t, hostile.ID, "b")
		for _, c := range []struct {
			name string
			want blobfs.Directory
		}{{"a", a}, {"b", b}} {
			got, err := s.store.ResolveDirectoryFrom(s.ctx, s.db, hostile.ID, c.name)
			if err != nil || !equalDirectory(got, c.want) {
				t.Errorf("ResolveDirectoryFrom(hostile, %q) = %+v, %v, want %+v", c.name, got, err, c.want)
			}
		}
		if n := s.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id = "+s.db.Dialect().Placeholder(1), hostile.ID); n != len(hostileNames)+2 {
			t.Errorf("%d directories under the hostile parent, want %d", n, len(hostileNames)+2)
		}
	})
}
