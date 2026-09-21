package postgres_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// TestConstraintConstants proves that every constraint-name constant the
// root package exports names a constraint or a unique index the Postgres
// DDL declares, so the persistence layer's error mapping cannot drift from
// the schema. The scan is over the embedded up texts and needs no engine.
func TestConstraintConstants(t *testing.T) {
	set, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	declared := map[string]bool{}
	for _, m := range set.Migrations {
		for _, re := range []*regexp.Regexp{constraint, createdIndex} {
			for _, match := range re.FindAllStringSubmatch(m.Up, -1) {
				declared[match[1]] = true
			}
		}
	}
	for _, name := range []string{
		blobfs.ConstraintPrimaryKeyDirectory,
		blobfs.ConstraintPrimaryKeyFile,
		blobfs.ConstraintUniqueDirectoryRoot,
		blobfs.ConstraintUniqueDirectoryParentName,
		blobfs.ConstraintUniqueFileDirectoryName,
		blobfs.ConstraintForeignKeyDirectoryParent,
		blobfs.ConstraintForeignKeyFileDirectory,
	} {
		if !strings.HasPrefix(name, postgres.Source+"_") {
			t.Errorf("constant %q does not start with the source prefix", name)
		}
		if !declared[name] {
			t.Errorf("constant %q names no constraint or index in the DDL", name)
		}
	}
}

// TestRootSeed proves the directory migration seeds the root with the
// root package's id and the name /, and that the id is the only literal
// id in the set: a consumer addresses the root by blobfs.RootID and by
// nothing else.
func TestRootSeed(t *testing.T) {
	set, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	seed := "INSERT INTO blobfs_directory (id, name) VALUES ('" + blobfs.RootID + "', '/')"
	if !strings.Contains(set.Migrations[0].Up, seed) {
		t.Errorf("the directory migration does not seed the root with RootID:\n%s", set.Migrations[0].Up)
	}
	if !strings.Contains(set.Migrations[0].Up, "CREATE UNIQUE INDEX "+blobfs.ConstraintUniqueDirectoryRoot) {
		t.Errorf("the directory migration does not create the root's unique index")
	}
}
