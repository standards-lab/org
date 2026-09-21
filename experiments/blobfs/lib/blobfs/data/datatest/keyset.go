package datatest

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// keysetFiles is the number of files the keyset group seeds: not a
// multiple of the page size, so the last page is partial.
const keysetFiles = 23

// keysetPage is the page size the keyset group walks with.
const keysetPage = 5

// sortIndex is the index a consumer's migration adds for a sort by
// created_at, which the keyset group creates for its second pass and
// drops again.
const sortIndex = "blobfs_ix_file_directory_created"

// keysetSorts are the sorts the keyset group walks: one term, two terms
// through the appended key, and three, in both directions.
var keysetSorts = []struct {
	name string
	sort []query.Sort
}{
	{"name", []query.Sort{{Field: "name"}}},
	{"name desc", []query.Sort{{Field: "name", Descending: true}}},
	{"created_at", []query.Sort{{Field: "created_at"}}},
	{"created_at desc", []query.Sort{{Field: "created_at", Descending: true}}},
	{"created_at, version", []query.Sort{{Field: "created_at"}, {Field: "version"}}},
	{"created_at desc, version desc", []query.Sort{{Field: "created_at", Descending: true}, {Field: "version", Descending: true}}},
	{"version, created_at", []query.Sort{{Field: "version"}, {Field: "created_at"}}},
}

// keyset checks the cursor walk through the variant against the
// baseline: every sort walked by cursor to the end returns the same rows
// in the same order on both stores, the same rows an offset walk
// returns, each exactly once; once without an index on created_at, as
// the library ships, and once with the index a consumer adds.
func (s *suite) keyset(t *testing.T) {
	dir := s.mkdir(t, "keyset-"+t.Name())
	// Files whose created_at ties in groups of three, whose version
	// alternates, and whose name order differs from their creation
	// order, so every sort term decides some pairs.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range keysetFiles {
		s.insertFileAt(t, dir.ID, fmt.Sprintf("f%02d.txt", (i*7)%keysetFiles), base.Add(time.Duration(i/3)*time.Minute), int64(1+i%2))
	}
	for i := range 9 {
		s.mkdirUnder(t, dir.ID, fmt.Sprintf("d%d", (i*4)%9))
	}
	t.Run("WithoutIndex", func(t *testing.T) { s.walks(t, dir.ID) })
	t.Run("WithIndex", func(t *testing.T) {
		if _, err := s.db.ExecContext(s.ctx, "CREATE INDEX "+sortIndex+" ON blobfs_file (directory_id, created_at)"); err != nil {
			t.Fatalf("create the sort index: %v", err)
		}
		defer func() {
			if _, err := s.db.ExecContext(s.ctx, "DROP INDEX "+sortIndex); err != nil {
				t.Errorf("drop the sort index: %v", err)
			}
		}()
		s.walks(t, dir.ID)
	})
}

// walks runs every sort over the files and the children of dir.
func (s *suite) walks(t *testing.T, dir string) {
	for _, c := range keysetSorts {
		t.Run("Files/"+c.name, func(t *testing.T) {
			got := walk(t, func(l data.Listing) (data.Page[blobfs.File], error) { return s.store.ListFiles(s.ctx, s.db, dir, l) }, c.sort, fileID)
			base := walk(t, func(l data.Listing) (data.Page[blobfs.File], error) { return s.standard.ListFiles(s.ctx, s.db, dir, l) }, c.sort, fileID)
			if !slices.Equal(got, base) {
				t.Errorf("the variant's cursor walk returned\n%v\nand the baseline's\n%v", got, base)
			}
			offset := byNumber(t, func(l data.Listing) (data.Page[blobfs.File], error) { return s.store.ListFiles(s.ctx, s.db, dir, l) }, c.sort, fileID)
			if !slices.Equal(got, offset) {
				t.Errorf("the cursor walk returned\n%v\nand the offset walk\n%v", got, offset)
			}
			if len(got) != keysetFiles || len(slices.Compact(slices.Sorted(slices.Values(got)))) != keysetFiles {
				t.Errorf("the walk returned %d rows with duplicates, want %d distinct", len(got), keysetFiles)
			}
		})
	}
	for _, c := range keysetSorts[:4] {
		t.Run("Children/"+c.name, func(t *testing.T) {
			got := walk(t, func(l data.Listing) (data.Page[blobfs.Directory], error) {
				return s.store.Children(s.ctx, s.db, dir, l)
			}, c.sort, directoryID)
			base := walk(t, func(l data.Listing) (data.Page[blobfs.Directory], error) {
				return s.standard.Children(s.ctx, s.db, dir, l)
			}, c.sort, directoryID)
			if !slices.Equal(got, base) {
				t.Errorf("the variant's cursor walk returned\n%v\nand the baseline's\n%v", got, base)
			}
			if len(got) != 9 || len(slices.Compact(slices.Sorted(slices.Values(got)))) != 9 {
				t.Errorf("the walk returned %d rows with duplicates, want 9 distinct", len(got))
			}
		})
	}
}

func fileID(f blobfs.File) string           { return f.Name + "@" + f.ID }
func directoryID(d blobfs.Directory) string { return d.Name + "@" + d.ID }

// walk reads page 1 with its total and then follows the cursor to the
// end, returning the keys of every row in order. Every page but the last
// reports More with a cursor, and the last neither.
func walk[T any](t *testing.T, list func(data.Listing) (data.Page[T], error), sort []query.Sort, key func(T) string) []string {
	t.Helper()
	var out []string
	l := data.Listing{Page: 1, Size: keysetPage, Sort: sort}
	for pages := 0; ; pages++ {
		page, err := list(l)
		if err != nil {
			t.Fatalf("page %d (%s): %v", pages+1, l.After, err)
		}
		if len(page.Rows) > keysetPage {
			t.Fatalf("page %d holds %d rows, more than the size", pages+1, len(page.Rows))
		}
		for _, row := range page.Rows {
			out = append(out, key(row))
		}
		if !page.More {
			if page.Next != "" {
				t.Errorf("the last page carries a cursor")
			}
			return out
		}
		if page.Next == "" {
			t.Fatalf("page %d reports more rows and no cursor under %v", pages+1, sort)
		}
		if pages > keysetFiles {
			t.Fatalf("the walk did not end after %d pages", pages)
		}
		l = data.Listing{Size: keysetPage, Sort: sort, After: page.Next, Total: data.TotalNone}
	}
}

// byNumber reads every page by its number and returns the keys of every
// row in order.
func byNumber[T any](t *testing.T, list func(data.Listing) (data.Page[T], error), sort []query.Sort, key func(T) string) []string {
	t.Helper()
	var out []string
	for n := 1; ; n++ {
		page, err := list(data.Listing{Page: n, Size: keysetPage, Sort: sort, Total: data.TotalNone})
		if err != nil {
			t.Fatalf("page %d by number: %v", n, err)
		}
		for _, row := range page.Rows {
			out = append(out, key(row))
		}
		if !page.More {
			return out
		}
	}
}

// insertFileAt inserts an available file row at a fixed created_at and
// version through plain SQL and returns its id.
func (s *suite) insertFileAt(t *testing.T, dir, name string, at time.Time, version int64) string {
	t.Helper()
	id := blobfs.NewID()
	p := s.db.Dialect().Placeholder
	text := fmt.Sprintf(
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size, version, created_at, updated_at) VALUES (%s, %s, %s, 'available', %s, 'text/plain', 1, %s, %s, %s)",
		p(1), p(2), p(3), p(4), p(5), p(6), p(6))
	if _, err := s.db.ExecContext(s.ctx, text, id, dir, name, id+"/"+name, version, at); err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}
