package schema_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/admin/schema"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// run executes the schema command with args over newClient and returns
// what it wrote to stdout and the error it returned.
func run(t *testing.T, newClient func() (*schema.Client, error), args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := schema.Commands(newClient, output.New(&out, &errOut))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestCommands_MountsUpAndDown(t *testing.T) {
	cmd := schema.Commands(nil, nil)
	if cmd.Name() != "schema" {
		t.Errorf("Commands().Name() = %q, want schema", cmd.Name())
	}
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	if got := strings.Join(names, ","); got != "down,up" {
		t.Errorf("schema subcommands = %s, want down,up", got)
	}
}

func TestCommands_ConstructsTheClientWhenALeafRuns(t *testing.T) {
	calls := 0
	newClient := func() (*schema.Client, error) {
		calls++
		pool, _ := sqltest.Open(t, append(setRun(3), setRun(2)...)...)
		return schema.NewClient(sqlate.Wrap(pool, postgresDialect{}), nil)
	}
	schema.Commands(newClient, output.New(&bytes.Buffer{}, &bytes.Buffer{}))
	if calls != 0 {
		t.Fatalf("Commands constructed the client %d times while building the tree", calls)
	}
	out, err := run(t, newClient, "up")
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if calls != 1 {
		t.Errorf("the client was constructed %d times, want once at run time", calls)
	}
	if !strings.HasPrefix(out, "schema up:") || strings.Count(out, "\n") != 1 {
		t.Errorf("stdout = %q, want one schema up line", out)
	}
}

// A constructor that fails, as the composition root's does when no DSN is
// set, returns its error from the leaf unrendered and prints no result.
func TestCommands_ReturnTheConstructorsError(t *testing.T) {
	want := errors.New("no database")
	failing := func() (*schema.Client, error) { return nil, want }
	for _, args := range [][]string{{"up"}, {"down"}} {
		out, err := run(t, failing, args...)
		if !errors.Is(err, want) {
			t.Errorf("%v: err = %v, want the constructor's error", args, err)
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want nothing", args, out)
		}
	}
}

func TestCommands_PrintHelpForTheContainerAndRefuseArguments(t *testing.T) {
	calls := 0
	counting := func() (*schema.Client, error) { calls++; return nil, errors.New("must not run") }
	out, err := run(t, counting)
	if err != nil {
		t.Errorf("schema: err = %v", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("schema: stdout = %q, want the help text", out)
	}
	for _, args := range [][]string{{"up", "x"}, {"down", "x"}, {"x"}} {
		if _, err := run(t, counting, args...); err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%v: err = %v, want cobra's unknown command error", args, err)
		}
	}
	if calls != 0 {
		t.Errorf("a container or a refused leaf constructed the client %d times", calls)
	}
}
