package volume_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
)

// TestEntityTags fixes the scan and binding contract: every exported field
// of the row and entry types carries a json tag naming its column, in the
// column order of its table or view, and only a nullable column is a
// pointer: a file's size and etag, which are NULL until the file is
// available.
func TestEntityTags(t *testing.T) {
	columns := map[string][]string{
		"Owner":       {"volume_id", "unit_id", "version", "created_at", "updated_at"},
		"Bookmark":    {"volume_id", "file_id", "active", "created_at", "updated_at"},
		"VolumeEntry": {"id", "name", "version", "created_at", "updated_at", "unit_id"},
		"FileEntry":   {"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at", "path", "volume_id", "unit_id"},
	}
	nullable := map[string]bool{"FileEntry.size": true, "FileEntry.etag": true}
	for _, v := range []any{volume.Owner{}, volume.Bookmark{}, volume.VolumeEntry{}, volume.FileEntry{}} {
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
			if f.Anonymous {
				t.Errorf("%s.%s is embedded; the mapper does not flatten an embedded struct", rt.Name(), f.Name)
			}
			if pointer := f.Type.Kind() == reflect.Pointer; pointer != nullable[rt.Name()+"."+tag] {
				t.Errorf("%s.%s pointer = %v, want %v", rt.Name(), f.Name, pointer, !pointer)
			}
		}
	}
}
