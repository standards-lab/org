package volume_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
)

// TestEntityTags fixes the scan and binding contract: every exported field
// of Owner and Bookmark carries a json tag naming its column, and no column
// of either table is nullable, so no field is a pointer.
func TestEntityTags(t *testing.T) {
	columns := map[string][]string{
		"Owner":    {"volume_id", "unit_id", "version", "created_at", "updated_at"},
		"Bookmark": {"volume_id", "file_id", "active", "created_at", "updated_at"},
	}
	for _, v := range []any{volume.Owner{}, volume.Bookmark{}} {
		rt := reflect.TypeOf(v)
		want := columns[rt.Name()]
		if rt.NumField() != len(want) {
			t.Errorf("%s has %d fields, want %d (%v)", rt.Name(), rt.NumField(), len(want), want)
			continue
		}
		for i := range rt.NumField() {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			if tag != want[i] || tag != strings.ToLower(tag) {
				t.Errorf("%s.%s has json tag %q, want %q", rt.Name(), f.Name, tag, want[i])
			}
			if f.Type.Kind() == reflect.Pointer {
				t.Errorf("%s.%s is a pointer; no column of %s is nullable", rt.Name(), f.Name, strings.ToLower(rt.Name()))
			}
		}
	}
}
