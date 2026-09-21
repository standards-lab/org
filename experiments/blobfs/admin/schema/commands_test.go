package schema_test

import (
	"bytes"
	"database/sql/driver"
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

func TestCommands_MountsTheFourSubcommands(t *testing.T) {
	cmd := schema.Commands(nil, nil)
	if cmd.Name() != "schema" {
		t.Errorf("Commands().Name() = %q, want schema", cmd.Name())
	}
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	if got := strings.Join(names, ","); got != "down,reset,status,up" {
		t.Errorf("schema subcommands = %s, want down,reset,status,up", got)
	}
}

func TestCommands_ConstructsTheClientWhenALeafRuns(t *testing.T) {
	calls := 0
	newClient := func() (*schema.Client, error) {
		calls++
		pool, _ := sqltest.Open(t, freshUp()...)
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
	for _, args := range [][]string{{"up"}, {"down"}, {"status"}, {"reset", "--yes"}} {
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
	for _, args := range [][]string{{"up", "x"}, {"down", "x"}, {"status", "x"}, {"reset", "x", "--yes"}, {"x"}} {
		if _, err := run(t, counting, args...); err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%v: err = %v, want cobra's unknown command error", args, err)
		}
	}
	if calls != 0 {
		t.Errorf("a container or a refused leaf constructed the client %d times", calls)
	}
}

// TestCommands_ResetRequiresYes proves reset refuses without --yes, before
// constructing the client, with the sentinel the root renders; with --yes
// it runs the client's Reset and prints its result line.
func TestCommands_ResetRequiresYes(t *testing.T) {
	calls := 0
	counting := func() (*schema.Client, error) { calls++; return nil, errors.New("must not run") }
	out, err := run(t, counting, "reset")
	if !errors.Is(err, schema.ErrResetNotConfirmed) {
		t.Errorf("reset without --yes: err = %v, want ErrResetNotConfirmed", err)
	}
	if out != "" || calls != 0 {
		t.Errorf("reset without --yes wrote %q and constructed the client %d times", out, calls)
	}

	// A Reset over a database with nothing applied: the lock, both checks
	// finding no history table, each set's create and empty history read,
	// its DROP TABLE, and the unlock.
	newClient := func() (*schema.Client, error) {
		pool, _ := sqltest.Open(t, locked, absent, absent, sqltest.Response{}, sqltest.Response{Columns: historyCols}, sqltest.Response{}, sqltest.Response{}, sqltest.Response{Columns: historyCols}, sqltest.Response{}, unlocked)
		return schema.NewClient(sqlate.Wrap(pool, postgresDialect{}), nil)
	}
	out, err = run(t, newClient, "reset", "--yes")
	if err != nil {
		t.Fatalf("reset --yes: %v", err)
	}
	if !strings.HasPrefix(out, "schema reset:") {
		t.Errorf("reset --yes stdout = %q, want the result line", out)
	}
}

// TestCommands_StatusRendersATable proves status renders one row per set
// under the header, the pending migrations listed by number and name.
func TestCommands_StatusRendersATable(t *testing.T) {
	present := sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(1)}}}
	head := sqltest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(2), true}}}
	applied := sqltest.Response{Columns: historyCols, Rows: [][]driver.Value{{int64(1), "directory", false}, {int64(2), "file", true}}}
	newClient := func() (*schema.Client, error) {
		pool, _ := sqltest.Open(t, present, head, present, applied, absent, absent)
		return schema.NewClient(sqlate.Wrap(pool, postgresDialect{}), nil)
	}
	out, err := run(t, newClient, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	got := lines(out)
	if len(got) != 3 || !strings.HasPrefix(got[0], "set ") {
		t.Fatalf("status stdout:\n%s", out)
	}
	if f := strings.Fields(got[1]); len(f) != 7 || f[0] != "blobfs" || f[1] != "blobfs_schema_version" || f[2] != "2" || f[3] != "3" || f[4] != "3" || f[5] != "file_created_index" || f[6] != "true" {
		t.Errorf("blobfs row = %q", got[1])
	}
	if f := strings.Fields(got[2]); len(f) != 9 || f[0] != "consumer" || f[1] != "schema_version" || f[2] != "0" || f[3] != "2" || f[4] != "1" || f[5] != "directory_owner," || f[6] != "2" || f[7] != "bookmark" || f[8] != "false" {
		t.Errorf("consumer row = %q", got[2])
	}
}

// lines splits stdout into its lines.
func lines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}
