package migrations_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// postgresDialect is the stub dialect under the name the Postgres engine
// reports, so the test selects the Postgres directory without the driver.
type postgresDialect struct{ sqltest.Dialect }

func (postgresDialect) Name() string { return "postgres" }

// TestMigrations loads the Postgres set and checks its shape: strictly
// increasing versions, a name and an up text on every migration, and a
// down text on every migration, so a consumer's Reset can revert the set.
func TestMigrations(t *testing.T) {
	set, err := migrations.Migrations(postgresDialect{})
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	if len(set) != 2 {
		t.Fatalf("Migrations returned %d migrations, want 2 (directory, file)", len(set))
	}
	for i, name := range []string{"directory", "file"} {
		if set[i].Name != name {
			t.Errorf("migration %d is %q, want %q", i+1, set[i].Name, name)
		}
	}
	last := 0
	for _, m := range set {
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

// TestUnsupportedEngine proves a dialect without a directory is refused
// with the sentinel and the engine's name in the message.
func TestUnsupportedEngine(t *testing.T) {
	_, err := migrations.Migrations(sqltest.Dialect{})
	if !errors.Is(err, migrations.ErrUnsupportedEngine) {
		t.Fatalf("Migrations(test dialect) = %v, want ErrUnsupportedEngine", err)
	}
	if !strings.Contains(err.Error(), `"test"`) {
		t.Errorf("error %q does not name the engine", err)
	}
}

var (
	createdTable = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	createdIndex = regexp.MustCompile(`(?i)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	constraint   = regexp.MustCompile(`(?i)\bCONSTRAINT\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// TestObjectNames scans every up text for the objects it creates (tables,
// indexes, and named constraints) and checks that each name starts with the
// source name and an underscore, and that Table does too.
func TestObjectNames(t *testing.T) {
	prefix := migrations.Source + "_"
	if !strings.HasPrefix(migrations.Table, prefix) {
		t.Errorf("Table %q does not start with %q", migrations.Table, prefix)
	}
	set, err := migrations.Migrations(postgresDialect{})
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	for _, m := range set {
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
