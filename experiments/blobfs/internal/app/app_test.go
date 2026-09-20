package app_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/internal/app"
)

// execute runs the app with args in place of the process's own and returns
// what it wrote to stdout and stderr and the exit code. The root command
// reads os.Args when no arguments are set on it, and the app exposes no
// other way in, so the test swaps os.Args for the run's duration.
func execute(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"blobfs"}, args...)
	t.Cleanup(func() { os.Args = saved })
	var out, errOut bytes.Buffer
	code = app.New(&out, &errOut).Run(context.Background())
	return out.String(), errOut.String(), code
}

func TestRun_PrintsUsageForHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {}, {"schema"}} {
		out, errOut, code := execute(t, args...)
		if code != 0 {
			t.Errorf("%v exited %d: %s", args, code, errOut)
		}
		for _, want := range []string{"Usage:", "schema", "--dsn"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: stdout lacks %q:\n%s", args, want, out)
			}
		}
		if errOut != "" {
			t.Errorf("%v: stderr = %q, want nothing", args, errOut)
		}
	}
}

func TestRun_MountsUpAndDownUnderSchema(t *testing.T) {
	out, _, code := execute(t, "schema", "--help")
	if code != 0 {
		t.Fatalf("schema --help exited %d", code)
	}
	for _, want := range []string{"Available Commands:", "up", "down"} {
		if !strings.Contains(out, want) {
			t.Errorf("schema help lacks %q:\n%s", want, out)
		}
	}
}

func TestRun_RendersAnUnknownCommandAndExitsOne(t *testing.T) {
	out, errOut, code := execute(t, "bogus")
	if code != 1 {
		t.Errorf("exited %d, want 1", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
	if !strings.Contains(errOut, `unknown command "bogus"`) {
		t.Errorf("stderr = %q, want cobra's unknown command message", errOut)
	}
}

// The DSN is resolved when the schema client is constructed, from the flag
// or the environment, and a run with neither fails before any I/O and names
// both routes.
func TestRun_RefusesSchemaUpWithoutADSN(t *testing.T) {
	t.Setenv("BLOBFS_DSN", "")
	for _, args := range [][]string{{"schema", "up"}, {"schema", "down"}} {
		out, errOut, code := execute(t, args...)
		if code != 1 {
			t.Errorf("%v exited %d, want 1", args, code)
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want nothing", args, out)
		}
		if !strings.Contains(errOut, "BLOBFS_DSN") || !strings.Contains(errOut, "--dsn") {
			t.Errorf("%v: stderr = %q, want an error naming --dsn and BLOBFS_DSN", args, errOut)
		}
	}
}

// A leaf refuses arguments, and the refusal is rendered to stderr like any
// other error.
func TestRun_RefusesArgumentsOnALeaf(t *testing.T) {
	_, errOut, code := execute(t, "schema", "up", "extra")
	if code != 1 {
		t.Errorf("exited %d, want 1", code)
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Errorf("stderr = %q, want cobra's unknown command message", errOut)
	}
}
