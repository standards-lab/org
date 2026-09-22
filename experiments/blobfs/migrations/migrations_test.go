package migrations_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/migrations"
)

var objectName = regexp.MustCompile(`(?i)\b(?:CREATE\s+TABLE|CREATE\s+(?:UNIQUE\s+)?INDEX|CONSTRAINT)\s+([A-Za-z_][A-Za-z0-9_]*)`)

// TestMigrations loads the set and checks its shape: strictly increasing
// versions, a down on every migration, and no object named as if blobfs
// owned it.
func TestMigrations(t *testing.T) {
	set, err := migrations.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	if len(set) != 2 {
		t.Fatalf("Migrations returned %d migrations, want 2 (directory_owner, bookmark)", len(set))
	}
	for i, name := range []string{"directory_owner", "bookmark"} {
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
		if m.Down == "" {
			t.Errorf("version %d %s has no down text", m.Version, m.Name)
		}
		for _, match := range objectName.FindAllStringSubmatch(m.Up, -1) {
			if strings.HasPrefix(match[1], "blobfs_") {
				t.Errorf("version %d %s creates %q, a name reserved for blobfs", m.Version, m.Name, match[1])
			}
		}
	}
}

// TestConstraintConstants proves that every constraint-name constant the
// consumer's domain exports for the bookmark table names a constraint or
// a unique index the consumer's DDL declares, so the domain's error
// mapping cannot drift from the schema, and that none carries the
// blobfs_ prefix. The scan is over the embedded up texts and needs no
// engine.
func TestConstraintConstants(t *testing.T) {
	set, err := migrations.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	declared := map[string]bool{}
	for _, m := range set {
		for _, match := range objectName.FindAllStringSubmatch(m.Up, -1) {
			declared[match[1]] = true
		}
	}
	for _, name := range []string{
		files.ConstraintPrimaryKeyBookmark,
		files.ConstraintForeignKeyBookmarkFile,
		files.ConstraintUniqueBookmarkActive,
	} {
		if strings.HasPrefix(name, "blobfs_") {
			t.Errorf("constant %q carries the prefix reserved for blobfs", name)
		}
		if !declared[name] {
			t.Errorf("constant %q names no constraint or index in the consumer's DDL", name)
		}
	}
}
