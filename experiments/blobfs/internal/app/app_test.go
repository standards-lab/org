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
		wants := []string{"Usage:", "schema", "mkdir", "ls", "mv", "--dsn", "--variant", "standard or pgnative"}
		if len(args) == 1 && args[0] == "schema" {
			wants = []string{"Usage:", "schema", "--dsn", "--variant"}
		}
		for _, want := range wants {
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

// The variant is resolved when the files store is constructed, from the
// flag or the environment, before the database opens: a name that is
// neither standard nor pgnative fails with no DSN set and names both
// accepted names and both routes. The schema commands never resolve it.
func TestRun_RefusesAnUnknownVariantBeforeAnyIO(t *testing.T) {
	t.Setenv("BLOBFS_DSN", "")
	t.Setenv("BLOBFS_VARIANT", "")
	for _, tc := range []struct {
		env  string
		args []string
	}{
		{"", []string{"--variant", "bogus", "ls", "/"}},
		{"bogus", []string{"ls", "/"}},
		{"bogus", []string{"put", "-", "/x.txt"}},
	} {
		t.Setenv("BLOBFS_VARIANT", tc.env)
		out, errOut, code := execute(t, tc.args...)
		if code != 1 {
			t.Errorf("%v (env %q) exited %d, want 1", tc.args, tc.env, code)
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want nothing", tc.args, out)
		}
		for _, want := range []string{`unknown variant "bogus"`, "--variant", "BLOBFS_VARIANT", "standard or pgnative"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("%v (env %q): stderr = %q, want %q", tc.args, tc.env, errOut, want)
			}
		}
	}
	// An accepted name gets past the variant to the missing DSN, so the
	// variant is resolved first and the standard default needs no flag.
	t.Setenv("BLOBFS_VARIANT", "")
	for _, args := range [][]string{{"ls", "/"}, {"--variant", "pgnative", "ls", "/"}, {"--variant", "standard", "ls", "/"}} {
		_, errOut, code := execute(t, args...)
		if code != 1 || !strings.Contains(errOut, "BLOBFS_DSN") {
			t.Errorf("%v exited %d with %q, want the missing DSN", args, code, errOut)
		}
	}
	t.Setenv("BLOBFS_VARIANT", "bogus")
	if _, errOut, _ := execute(t, "schema", "up"); strings.Contains(errOut, "variant") {
		t.Errorf("schema up resolved the variant: %q", errOut)
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
