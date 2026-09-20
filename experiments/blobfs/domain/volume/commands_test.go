package volume_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// run mounts the domain's commands on a root and executes args over
// newStore, returning what was written to stdout and the error returned.
func run(t *testing.T, newStore func() (*volume.Store, error), args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "blobfs", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(volume.Commands(newStore, output.New(&out, &errOut))...)
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// counting returns a store constructor that counts its calls and fails, so
// a test can tell whether a leaf reached the store.
func counting(calls *int) func() (*volume.Store, error) {
	return func() (*volume.Store, error) {
		*calls++
		return nil, errors.New("must not run")
	}
}

func TestCommands_MountsVolumeMkdirAndLs(t *testing.T) {
	cmds := volume.Commands(nil, nil)
	var names []string
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	slices.Sort(names)
	if got := strings.Join(names, ","); got != "ls,mkdir,volume" {
		t.Errorf("Commands() = %s, want ls,mkdir,volume", got)
	}
	var subs []string
	for _, c := range cmds {
		if c.Name() == "volume" {
			for _, s := range c.Commands() {
				subs = append(subs, s.Name())
			}
		}
	}
	if got := strings.Join(subs, ","); got != "create,ls,rename" {
		t.Errorf("volume subcommands = %s, want create,ls,rename", got)
	}
}

// The flag and argument checks run before the store is constructed: a
// missing or malformed --unit, a malformed --sort, a bad address, and a
// mkdir of a root or of a path ending with a slash all fail without a store.
func TestCommands_ValidateBeforeConstructingTheStore(t *testing.T) {
	calls := 0
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"volume", "create", "docs"}, `required flag(s) "unit" not set`},
		{[]string{"volume", "create", "docs", "--unit", "not-a-uuid"}, `--unit "not-a-uuid" is not a UUID`},
		{[]string{"volume", "ls", "--unit", "123"}, `--unit "123" is not a UUID`},
		{[]string{"volume", "ls", "--sort", "name:sideways"}, `the direction is asc or desc`},
		{[]string{"volume", "ls", "--sort", ":desc"}, `names no field`},
		{[]string{"ls", "docs"}, `has no colon`},
		{[]string{"ls", "docs:/a", "--unit", "x"}, `is not a UUID`},
		{[]string{"ls", "docs:/a", "--sort", "path:up"}, `the direction is asc or desc`},
		{[]string{"mkdir", "docs:/"}, `is the volume's root`},
		{[]string{"mkdir", "docs:/a/"}, `ends with a slash`},
		{[]string{"mkdir", "docs"}, `has no colon`},
		{[]string{"mkdir"}, `accepts 1 arg`},
		{[]string{"volume", "rename", "docs"}, `accepts 2 arg`},
		{[]string{"volume", "create", "a", "b", "--unit", "0193b0a2-1111-7000-8000-000000000001"}, `accepts 1 arg`},
	} {
		out, err := run(t, counting(&calls), tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want nothing", tc.args, out)
		}
	}
	if calls != 0 {
		t.Errorf("a refused invocation constructed the store %d times", calls)
	}
}

// A constructor that fails, as the composition root's does when no DSN is
// set, returns its error unrendered from every leaf, after the flag checks.
func TestCommands_ReturnTheConstructorsError(t *testing.T) {
	want := errors.New("no database")
	failing := func() (*volume.Store, error) { return nil, want }
	unit := "0193b0a2-1111-7000-8000-000000000001"
	for _, args := range [][]string{
		{"volume", "create", "docs", "--unit", unit},
		{"volume", "ls"},
		{"volume", "ls", "--unit", unit, "--sort", "name:desc", "--page", "2", "--size", "5"},
		{"volume", "rename", "docs", "manuals"},
		{"mkdir", "docs:/a"},
		{"ls", "docs:/"},
	} {
		out, err := run(t, failing, args...)
		if !errors.Is(err, want) {
			t.Errorf("%v: err = %v, want the constructor's error", args, err)
		}
		if out != "" {
			t.Errorf("%v: stdout = %q, want nothing", args, out)
		}
	}
}

// The store is verified before a leaf's first use: a database that lacks
// the schema fails with ErrVerify, and its message names schema up.
func TestCommands_VerifyTheStoreBeforeUse(t *testing.T) {
	newStore := func() (*volume.Store, error) {
		s, rec := newStore(t)
		rec.FailPrepare = func(string) error { return errors.New("relation does not exist") }
		return s, nil
	}
	_, err := run(t, newStore, "volume", "ls")
	if !errors.Is(err, volume.ErrVerify) || !strings.Contains(err.Error(), "blobfs schema up") {
		t.Errorf("volume ls against an unverifiable database = %v, want ErrVerify naming schema up", err)
	}
}

func TestCommands_PrintHelpForTheContainer(t *testing.T) {
	calls := 0
	out, err := run(t, counting(&calls), "volume")
	if err != nil || !strings.Contains(out, "Usage:") || !strings.Contains(out, "rename") {
		t.Errorf("volume: out = %q, err = %v, want the help text", out, err)
	}
	if calls != 0 {
		t.Errorf("the container constructed the store %d times", calls)
	}
}

func TestParseSort(t *testing.T) {
	for in, want := range map[string]volume.Sort{
		"name":            {Field: "name"},
		"name:asc":        {Field: "name"},
		"name:desc":       {Field: "name", Descending: true},
		"created_at:desc": {Field: "created_at", Descending: true},
	} {
		got, err := volume.ParseSort(in)
		if err != nil || got != want {
			t.Errorf("ParseSort(%q) = %+v, %v, want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", ":desc", "name:", "name:down", "name:DESC"} {
		if _, err := volume.ParseSort(in); err == nil {
			t.Errorf("ParseSort(%q) succeeded, want an error", in)
		}
	}
}
