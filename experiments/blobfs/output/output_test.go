package output_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

func TestLine_WritesOneLineToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Line("schema up: applied")
	out.Line("second")
	if want := "schema up: applied\nsecond\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestError_WritesTheMessageToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := output.New(&stdout, &stderr)
	out.Error(errors.New("no database: set --dsn or BLOBFS_DSN"))
	if want := "no database: set --dsn or BLOBFS_DSN\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}
