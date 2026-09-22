package data

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestCursorEncoding proves the encoding round-trips, the value text
// round-trips a timestamp at full precision and an integer, and the
// nullable index of each entity is the pointer fields less the key.
func TestCursorEncoding(t *testing.T) {
	c := cursor{Listing: "files_in_directory", Sort: []term{{Field: "created_at", Descending: true}, {Field: "name", Descending: true}}, Values: []string{"2026-09-20T08:30:00.123456Z", "b.txt"}}
	s, err := encode(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(s, "+/=") {
		t.Errorf("the cursor %q is not base64url without padding", s)
	}
	back, err := decode(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	c.Version = cursorVersion
	if !reflect.DeepEqual(back, c) {
		t.Errorf("decode(encode(c)) = %+v, want %+v", back, c)
	}

	at := time.Date(2026, 9, 20, 10, 30, 0, 123456000, time.FixedZone("plus2", 2*3600))
	values, err := valuesOf(blobfs.File{Name: "x", Version: 42, CreatedAt: at}, []term{{Field: "created_at"}, {Field: "version"}, {Field: "name"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-09-20T08:30:00.123456Z", "42", "x"}; !reflect.DeepEqual(values, want) {
		t.Errorf("valuesOf = %v, want %v", values, want)
	}
	parsed, err := time.Parse(time.RFC3339Nano, values[0])
	if err != nil || !parsed.Equal(at) {
		t.Errorf("the timestamp text %q does not parse back to %v: %v", values[0], at, err)
	}
	if _, err := valuesOf(blobfs.File{}, []term{{Field: "size"}}); err == nil {
		t.Error("valuesOf over a NULL size succeeded, want a refusal")
	}

	if got := nullableOf[blobfs.File]("name"); !reflect.DeepEqual(got, map[string]bool{"size": true, "etag": true}) {
		t.Errorf("nullable file fields = %v, want size and etag", got)
	}
	if got := nullableOf[blobfs.Directory]("name"); !reflect.DeepEqual(got, map[string]bool{"parent_id": true}) {
		t.Errorf("nullable directory fields = %v, want parent_id alone (name is the key)", got)
	}
}

// craft encodes a body as a cursor with a valid checksum, so a test can
// present a well-formed cursor with content the listing never issues.
func craft(t *testing.T, body string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	return base64.RawURLEncoding.EncodeToString(append(sum[:checksumLen], body...))
}

// TestCraftedCursorsAreRefused proves the checks a well-formed cursor
// still meets: a value that does not parse as its field's type (uuid,
// bigint, timestamp), a version the listing does not issue, a body that
// is not JSON, a term count that does not match the values, and a
// listing name the store does not have. None reaches the driver.
func TestCraftedCursorsAreRefused(t *testing.T) {
	c, err := query.NewCatalog(query.Patterns(), Patterns())
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(c, sqltest.Dialect{})
	if err != nil {
		t.Fatal(err)
	}
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	cases := []struct {
		label  string
		body   string
		sort   []query.Sort
		reason string
	}{
		{"not a uuid", `{"v":1,"listing":"files_in_directory","sort":[{"field":"id"},{"field":"name"}],"values":["nope","x"]}`, []query.Sort{{Field: "id"}}, "value for id: not a uuid"},
		{"not an integer", `{"v":1,"listing":"files_in_directory","sort":[{"field":"version"},{"field":"name"}],"values":["1.5","x"]}`, []query.Sort{{Field: "version"}}, "value for version: not an integer"},
		{"not a timestamp", `{"v":1,"listing":"files_in_directory","sort":[{"field":"created_at"},{"field":"name"}],"values":["yesterday","x"]}`, []query.Sort{{Field: "created_at"}}, "value for created_at: not a timestamp"},
		{"another version", `{"v":2,"listing":"files_in_directory","sort":[{"field":"name"}],"values":["x"]}`, nil, "version 2"},
		{"not json", `name>x`, nil, "malformed body"},
		{"values do not match", `{"v":1,"listing":"files_in_directory","sort":[{"field":"name"}],"values":["x","y"]}`, nil, "do not match"},
		{"no terms", `{"v":1,"listing":"files_in_directory","sort":[],"values":[]}`, nil, "do not match"},
		{"unknown listing", `{"v":1,"listing":"everything","sort":[{"field":"name"}],"values":["x"]}`, nil, "issued by everything"},
	}
	for _, tc := range cases {
		_, err := s.ListFiles(ctx, db, "dir", Listing{Size: 2, After: craft(t, tc.body), Sort: tc.sort})
		var ce *CursorError
		if !errors.As(err, &ce) || !errors.Is(err, query.ErrDirectives) || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("%s = %v, want a CursorError with %q", tc.label, err, tc.reason)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("crafted cursors reached the driver with %d calls", n)
	}
}
