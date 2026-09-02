package schema_test

import (
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/internal/schema"
)

// The embedded set parses, orders, and declares its transaction modes as
// authored: the index build runs outside a transaction.
func TestMigrations_EmbeddedSetParses(t *testing.T) {
	ms := schema.Migrations()
	if len(ms) != 3 {
		t.Fatalf("len = %d, want 3", len(ms))
	}
	want := []struct {
		version       int
		name          string
		transactional bool
	}{{1, "organization", true}, {2, "person", true}, {3, "person_unit_index", false}}
	for i, w := range want {
		if ms[i].Version != w.version || ms[i].Name != w.name || ms[i].Transactional != w.transactional {
			t.Errorf("[%d] = %d %s tx=%v, want %+v", i, ms[i].Version, ms[i].Name, ms[i].Transactional, w)
		}
		if ms[i].Down == "" {
			t.Errorf("[%d] has no down", i)
		}
	}
}
