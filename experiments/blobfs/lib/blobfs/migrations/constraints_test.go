package migrations_test

import (
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// TestConstraintConstants proves that every constraint-name constant the
// root package exports names a constraint the Postgres DDL declares, so
// the persistence layer's error mapping cannot drift from the schema. The
// scan is over the embedded up texts and needs no engine.
func TestConstraintConstants(t *testing.T) {
	set, err := migrations.Migrations(postgresDialect{})
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	declared := map[string]bool{}
	for _, m := range set {
		for _, match := range constraint.FindAllStringSubmatch(m.Up, -1) {
			declared[match[1]] = true
		}
	}
	for _, name := range []string{
		blobfs.ConstraintUniqueVolumeName,
		blobfs.ConstraintUniqueDirectoryVolume,
		blobfs.ConstraintUniqueDirectoryParentName,
		blobfs.ConstraintUniqueFileDirectoryName,
		blobfs.ConstraintForeignKeyDirectoryParent,
		blobfs.ConstraintForeignKeyDirectoryVolume,
		blobfs.ConstraintForeignKeyFileDirectory,
	} {
		if !strings.HasPrefix(name, migrations.Source+"_") {
			t.Errorf("constant %q does not start with the source prefix", name)
		}
		if !declared[name] {
			t.Errorf("constant %q names no constraint in the DDL", name)
		}
	}
}
