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
		output.Page{Number: 1, Size: 20, Listed: 2, Total: 7, Counted: true},
	)
	want := "KIND  NAME         SIZE  STATUS     UPDATED\n" +
		"dir   reports      -     -          2026-09-20 10:30:00\n" +
		"file  a.txt        1234  available  2026-09-20 10:30:00\n" +
		"file  pending.bin  -     pending    2026-09-20 10:30:00\n" +
		"directories: 1 on page 1 of size 20, total 1\n" +
		"files: 2 on page 1 of size 20, total 7\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

// A half whose total was not asked for says so, and an empty page after
// the first, which carries no total, is reported as unknown rather than
// as zero.
func TestListing_SaysWhenATotalIsAbsent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Listing(nil,
		output.Page{Number: 3, Size: 5, Listed: 0, Total: output.NoTotal, Counted: true},
		output.Page{Number: 3, Size: 5, Listed: 0, Counted: false},
	)
	want := "KIND  NAME  SIZE  STATUS  UPDATED\n" +
		"directories: 0 on page 3 of size 5, total unknown (the page is empty)\n" +
		"files: 0 on page 3 of size 5, total not counted\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}
