package postgres_test

import (
	"context"
	"database/sql/driver"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// fileRowAt scripts one available file row at a fixed time and version.
func fileRowAt(id, name string, at time.Time, version int64) []driver.Value {
	return []driver.Value{id, blobfs.RootID, name, "available", id + "/" + name, nil, "text/plain", nil, version, at, at}
}

// files scripts one response of the plain file listing with one row per
// name, all at the same time and version.
func files(at time.Time, version int64, names ...string) sqltest.Response {
	resp := sqltest.Response{Columns: fileColumns}
	for _, name := range names {
		resp.Rows = append(resp.Rows, fileRowAt("id-"+name, name, at, version))
	}
	return resp
}

// TestKeysetRowValue proves the Postgres variant's cursor predicate as
// composed: one term keeps the baseline's scalar comparison, and two and
// three terms render the row-value comparison with every value cast to
// its type and bound once, in term order, > under an ascending sort and
// < under a descending one, with the placeholder numbering carried on
// to the paging clause and the bound values in the same order.
func TestKeysetRowValue(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 10, 30, 0, 123456000, time.FixedZone("plus2", 2*3600))
	utc := "2026-09-20T08:30:00.123456Z"
	for _, c := range []struct {
		name string
		sort []query.Sort
		want string
		args []any
	}{
		{
			"OneTermAscending",
			[]query.Sort{{Field: "name"}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND q.name > CAST($2 AS text) ORDER BY q.name OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY",
			[]any{"dir", "b.txt", 0, 3},
		},
		{
			"OneTermDescending",
			[]query.Sort{{Field: "name", Descending: true}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND q.name < CAST($2 AS text) ORDER BY q.name DESC OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY",
			[]any{"dir", "b.txt", 0, 3},
		},
		{
			"TwoTermsAscending",
			[]query.Sort{{Field: "created_at"}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND (q.created_at, q.name) > (CAST($2 AS timestamp with time zone), CAST($3 AS text)) ORDER BY q.created_at, q.name OFFSET $4 ROWS FETCH NEXT $5 ROWS ONLY",
			[]any{"dir", utc, "b.txt", 0, 3},
		},
		{
			"TwoTermsDescending",
			[]query.Sort{{Field: "created_at", Descending: true}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND (q.created_at, q.name) < (CAST($2 AS timestamp with time zone), CAST($3 AS text)) ORDER BY q.created_at DESC, q.name DESC OFFSET $4 ROWS FETCH NEXT $5 ROWS ONLY",
			[]any{"dir", utc, "b.txt", 0, 3},
		},
		{
			"ThreeTermsAscending",
			[]query.Sort{{Field: "created_at"}, {Field: "version"}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND (q.created_at, q.version, q.name) > (CAST($2 AS timestamp with time zone), CAST($3 AS bigint), CAST($4 AS text)) ORDER BY q.created_at, q.version, q.name OFFSET $5 ROWS FETCH NEXT $6 ROWS ONLY",
			[]any{"dir", utc, "7", "b.txt", 0, 3},
		},
		{
			"ThreeTermsDescending",
			[]query.Sort{{Field: "created_at", Descending: true}, {Field: "version", Descending: true}},
			"WHERE q.directory_id = CAST($1 AS uuid) AND (q.created_at, q.version, q.name) < (CAST($2 AS timestamp with time zone), CAST($3 AS bigint), CAST($4 AS text)) ORDER BY q.created_at DESC, q.version DESC, q.name DESC OFFSET $5 ROWS FETCH NEXT $6 ROWS ONLY",
			[]any{"dir", utc, "7", "b.txt", 0, 3},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, db, rec := openNative(t, files(at, 7, "a.txt", "b.txt", "c.txt"), files(at, 7, "c.txt"))
			l := data.Listing{Size: 2, Page: 1, Total: data.TotalNone, Sort: c.sort}
			page, err := s.ListFiles(ctx, db, "dir", l)
			if err != nil || page.Next == "" {
				t.Fatalf("page 1 = %+v, %v; want a cursor", page, err)
			}
			l.After = page.Next
			if _, err := s.ListFiles(ctx, db, "dir", l); err != nil {
				t.Fatalf("page 2: %v", err)
			}
			got := rec.SQL(sqltest.OpQuery)[1]
			if !strings.HasSuffix(got, c.want) {
				t.Errorf("the cursor page composed:\n%s\nwant the suffix\n%s", got, c.want)
			}
			if args := rec.Calls()[1].Args; !slices.Equal(args, c.args) {
				t.Errorf("the cursor page bound %v, want %v", args, c.args)
			}
		})
	}
}

// TestVerifyPreparesTheRowValueRenderings proves the store's Verify over
// the Postgres variant prepares the cursor renderings in the row-value
// form, one per listing over the fields a cursor can continue, so a
// column the schema no longer has fails at startup on this variant as on
// the baseline.
func TestVerifyPreparesTheRowValueRenderings(t *testing.T) {
	s, err := data.New(catalog(t), sqltest.Dialect{}, data.WithVariant(newVariant(t)))
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	pool, rec := sqltest.Open(t)
	if err := s.Verify(context.Background(), sqlate.Wrap(pool, sqltest.Dialect{})); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	wantFiles := "WHERE q.directory_id = CAST($1 AS uuid) AND (q.id, q.directory_id, q.name) > (CAST($2 AS uuid), CAST($3 AS uuid), CAST($4 AS text)) ORDER BY q.id, q.directory_id, q.name, q.status, q.content_type, q.version, q.created_at, q.updated_at OFFSET $5 ROWS FETCH NEXT $6 ROWS ONLY"
	wantChildren := "WHERE q.parent_id = CAST($1 AS uuid) AND (q.id, q.name) > (CAST($2 AS uuid), CAST($3 AS text)) ORDER BY q.id, q.name, q.version, q.created_at, q.updated_at OFFSET $4 ROWS FETCH NEXT $5 ROWS ONLY"
	found, chains := 0, 0
	for _, text := range rec.SQL(sqltest.OpPrepare) {
		if strings.HasSuffix(text, wantFiles) || strings.HasSuffix(text, wantChildren) {
			found++
		}
		if strings.Contains(text, " OR (") {
			chains++
		}
	}
	if found != 2 || chains != 0 {
		t.Errorf("Verify prepared %d row-value cursor renderings and %d expanded ones, want 2 and 0:\n%s", found, chains, strings.Join(rec.SQL(sqltest.OpPrepare), "\n"))
	}
}
