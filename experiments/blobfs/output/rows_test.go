package output_test

import (
	"bytes"
	"testing"

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
