package output_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

func TestRows_AlignsColumnsUnderTheHeader(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Rows([]string{"NAME", "VERSION", "UNIT"}, [][]string{
		{"docs", "1", "0193b0a2-1111-7000-8000-000000000001"},
		{"a much longer name", "12", "-"},
	})
	out.Total(2)
	want := "NAME                VERSION  UNIT\n" +
		"docs                1        0193b0a2-1111-7000-8000-000000000001\n" +
		"a much longer name  12       -\n" +
		"total: 2\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestRows_WritesTheHeaderForAnEmptyListing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Rows([]string{"KIND", "NAME"}, nil)
	out.Total(0)
	if want := "KIND  NAME\ntotal: 0\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestListing_WritesEntriesThenOneLinePerHalf(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	size := int64(1234)
	at := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	out.Listing([]output.Entry{
		{Kind: "dir", Name: "reports", Updated: at},
		{Kind: "file", Name: "a.txt", Size: &size, Status: "available", Updated: at},
		{Kind: "file", Name: "pending.bin", Status: "pending", Updated: at},
	},
		output.Page{Number: 1, Size: 20, Listed: 1, Total: 1, Counted: true},
		output.Page{Number: 1, Size: 20, Listed: 2, Total: 7, Counted: true, More: true},
	)
	want := "KIND  NAME         SIZE  STATUS     UPDATED\n" +
		"dir   reports      -     -          2026-09-20 10:30:00\n" +
		"file  a.txt        1234  available  2026-09-20 10:30:00\n" +
		"file  pending.bin  -     pending    2026-09-20 10:30:00\n" +
		"directories: 1 on page 1 of size 20, total 1\n" +
		"more: no\n" +
		"files: 2 on page 1 of size 20, total 7\n" +
		"more: yes\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

// A half whose total was not asked for says so, and an empty page after
// the first, which carries no total, is reported as unknown rather than
// as zero. The more: line does not depend on the total: a half without
// one still says whether rows remain.
func TestListing_SaysWhenATotalIsAbsent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Listing(nil,
		output.Page{Number: 3, Size: 5, Listed: 0, Total: output.NoTotal, Counted: true},
		output.Page{Number: 3, Size: 5, Listed: 0, Counted: false, More: true},
	)
	want := "KIND  NAME  SIZE  STATUS  UPDATED\n" +
		"directories: 0 on page 3 of size 5, total unknown (the page is empty)\n" +
		"more: no\n" +
		"files: 0 on page 3 of size 5, total not counted\n" +
		"more: yes\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}

// A half read after a cursor says so instead of naming a page, and a half
// with a next page writes its cursor on a line of its own under the label
// its flag takes, next-dirs or next-files, after its more: line.
func TestListing_WritesTheCursorLines(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Listing(nil,
		output.Page{Number: 1, Size: 2, Listed: 2, Total: 5, Counted: true, More: true, Next: "DIRS"},
		output.Page{Size: 2, Listed: 2, Total: output.NoTotal, Cursor: true, More: true, Next: "FILES"},
	)
	want := "KIND  NAME  SIZE  STATUS  UPDATED\n" +
		"directories: 2 on page 1 of size 2, total 5\n" +
		"more: yes\n" +
		"next-dirs: DIRS\n" +
		"files: 2 after the cursor, size 2, total not counted\n" +
		"more: yes\n" +
		"next-files: FILES\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}

func TestRecord_AlignsValuesAfterTheLabels(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Record([]output.Field{{Name: "path", Value: "/a/b.txt"}, {Name: "content-type", Value: "text/plain"}, {Name: "size", Value: "-"}})
	want := "path:         /a/b.txt\n" +
		"content-type: text/plain\n" +
		"size:         -\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestCopy_StreamsTheBytesAsTheyAre(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	n, err := out.Copy(bytes.NewReader([]byte("no newline\x00\xff")))
	if err != nil || n != 12 {
		t.Fatalf("Copy = %d, %v", n, err)
	}
	if got := stdout.String(); got != "no newline\x00\xff" {
		t.Errorf("stdout = %q", got)
	}
}

// A bookmark listing writes the entries with the active marker and one
// line for the page, in the form a directory listing's halves use.
func TestBookmarks_WritesEntriesThenOneLine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	size := int64(42)
	at := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	out.Bookmarks([]output.BookmarkEntry{
		{Path: "/reports/2026/plan.txt", Size: &size, Status: "available", Active: true, Updated: at},
		{Path: "/draft.bin", Status: "pending", Updated: at},
	}, output.Page{Number: 1, Size: 20, Listed: 2, Total: 2, Counted: true})
	want := "PATH                    SIZE  STATUS     ACTIVE  UPDATED\n" +
		"/reports/2026/plan.txt  42    available  active  2026-09-20 10:30:00\n" +
		"/draft.bin              -     pending    -       2026-09-20 10:30:00\n" +
		"bookmarks: 2 on page 1 of size 20, total 2\n" +
		"more: no\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
	stdout.Reset()
	out.Bookmarks(nil, output.Page{Number: 1, Size: 20, Counted: false})
	if want := "PATH  SIZE  STATUS  ACTIVE  UPDATED\nbookmarks: 0 on page 1 of size 20, total not counted\nmore: no\n"; stdout.String() != want {
		t.Errorf("an empty listing without a total =\n%s\nwant\n%s", stdout.String(), want)
	}
}
