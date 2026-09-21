package postgres_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// TestMigrations loads the set and checks its shape: the name and the
// history table the package exports, strictly increasing versions, a name
// and an up text on every migration, and a down text on every migration,
// so a consumer's Reset can revert the set.
func TestMigrations(t *testing.T) {
	set, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	if set.Name != postgres.Source || set.Table != postgres.Table {
		t.Errorf("set = %q under %q, want %q under %q", set.Name, set.Table, postgres.Source, postgres.Table)
	}
	if len(set.Migrations) != 2 {
		t.Fatalf("Migrations returned %d migrations, want 2 (directory, file)", len(set.Migrations))
	}
	for i, name := range []string{"directory", "file"} {
		if set.Migrations[i].Name != name {
			t.Errorf("migration %d is %q, want %q", i+1, set.Migrations[i].Name, name)
		}
	}
	last := 0
	for _, m := range set.Migrations {
		if m.Version <= last {
			t.Errorf("version %d follows %d; versions must strictly increase", m.Version, last)
		}
		last = m.Version
		if m.Name == "" || m.Up == "" {
			t.Errorf("version %d has no name or no up text", m.Version)
		}
		if m.Down == "" {
			t.Errorf("version %d %s has no down text", m.Version, m.Name)
		}
	}
}

var (
	createdTable = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	createdIndex = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	constraint   = regexp.MustCompile(`(?i)\bCONSTRAINT\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// TestObjectNames scans every up text for the objects it creates (tables,
// indexes, and named constraints) and checks that each name starts with the
// set's name and an underscore, and that Table does too.
func TestObjectNames(t *testing.T) {
	prefix := postgres.Source + "_"
	if !strings.HasPrefix(postgres.Table, prefix) {
		t.Errorf("Table %q does not start with %q", postgres.Table, prefix)
	}
	set, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	for _, m := range set.Migrations {
		var names []string
		for _, re := range []*regexp.Regexp{createdTable, createdIndex, constraint} {
			for _, match := range re.FindAllStringSubmatch(m.Up, -1) {
				names = append(names, match[1])
			}
		}
		if len(names) == 0 {
			t.Errorf("version %d %s creates no named object", m.Version, m.Name)
		}
		for _, name := range names {
			if !strings.HasPrefix(name, prefix) {
				t.Errorf("version %d %s creates %q, which does not start with %q", m.Version, m.Name, name, prefix)
			}
		}
	}
}
