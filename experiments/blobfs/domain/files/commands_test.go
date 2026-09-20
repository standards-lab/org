package files_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// run mounts the domain's commands on a root and executes args over
// newStore, returning what was written to stdout and the error returned.
func run(t *testing.T, newStore func() (*files.Store, error), args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "blobfs", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(files.Commands(newStore, output.New(&out, &errOut))...)
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// counting returns a store constructor that counts its calls and fails, so
// a test can tell whether a leaf reached the store.
func counting(calls *int) func() (*files.Store, error) {
	return func() (*files.Store, error) {
		*calls++
		return nil, errors.New("must not run")
	}
}

const unit = "0193b0a2-1111-7000-8000-000000000001"

func TestCommands_MountsMkdirAndLs(t *testing.T) {
	cmds := files.Commands(nil, nil)
	var names []string
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	slices.Sort(names)
	if got := strings.Join(names, ","); got != "ls,mkdir" {
		t.Errorf("Commands() = %s, want ls,mkdir", got)
	}
}

// The flag and argument checks run before the store is constructed: a
// malformed --unit, a malformed --sort, a bad --total, and a wrong
// argument count all fail without a store.
func TestCommands_ValidateBeforeConstructingTheStore(t *testing.T) {
	calls := 0
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"mkdir", "/docs", "--unit", "not-a-uuid"}, `--unit "not-a-uuid" is not a UUID`},
		{[]string{"ls", "/", "--unit", "123"}, `--unit "123" is not a UUID`},
		{[]string{"ls", "/", "--sort", "name:sideways"}, `the direction is asc or desc`},
		{[]string{"ls", "/", "--sort", ":desc"}, `names no field`},
		{[]string{"ls", "/", "--total", "estimate"}, `the mode is exact or none`},
		{[]string{"ls"}, `accepts 1 arg`},
		{[]string{"mkdir"}, `accepts 1 arg`},
		{[]string{"mkdir", "/a", "/b"}, `accepts 1 arg`},
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
	failing := func() (*files.Store, error) { return nil, want }
	for _, args := range [][]string{
		{"mkdir", "/docs"},
		{"mkdir", "/docs", "--unit", unit},
		{"ls", "/"},
		{"ls", "/docs", "--unit", unit, "--sort", "name:desc", "--page", "2", "--size", "5", "--total", "none"},
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
	unverifiable := func() (*files.Store, error) {
		s, rec := newStore(t)
		rec.FailPrepare = func(string) error { return errors.New("relation does not exist") }
		return s, nil
	}
	_, err := run(t, unverifiable, "ls", "/")
	if !errors.Is(err, files.ErrVerify) || !strings.Contains(err.Error(), "blobfs schema up") {
		t.Errorf("ls against an unverifiable database = %v, want ErrVerify naming schema up", err)
	}
}

// TestCommands_RenderTheListing proves ls renders directories then files
// with a line per half, and that the flags reach the store: page, size,
// and the total mode.
func TestCommands_RenderTheListing(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 3, "docs"), listing(fileColumns, true, 9, "a.txt", "b.txt"))
		return s, nil
	}
	out, err := run(t, scripted, "ls", "/", "--page", "2", "--size", "2")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 6 || !strings.HasPrefix(lines[0], "KIND") || !strings.HasPrefix(lines[1], "dir   docs") || !strings.HasPrefix(lines[2], "file  a.txt  1") || !strings.HasPrefix(lines[3], "file  b.txt  2") {
		t.Errorf("stdout =\n%s", out)
	}
	if lines[4] != "directories: 1 on page 2 of size 2, total 3" || lines[5] != "files: 2 on page 2 of size 2, total 9" {
		t.Errorf("half lines = %q, %q", lines[4], lines[5])
	}

	// An empty page after the first has no total, and --total none never
	// asks for one.
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--page", "3")
	if err != nil {
		t.Fatalf("ls page 3: %v", err)
	}
	if !strings.Contains(out, "directories: 0 on page 3 of size 20, total unknown") || !strings.Contains(out, "files: 0 on page 3 of size 20, total unknown") {
		t.Errorf("an empty later page rendered:\n%s", out)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, false, 0, "docs"), listing(fileColumns, false, 0))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--total", "none")
	if err != nil {
		t.Fatalf("ls --total none: %v", err)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total not counted") || !strings.Contains(out, "files: 0 on page 1 of size 20, total not counted") {
		t.Errorf("--total none rendered:\n%s", out)
	}
}

// TestCommands_RenderMkdir proves mkdir prints one result line naming the
// path and the id, and the unit when one was given.
func TestCommands_RenderMkdir(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), affected(), directory("A", "root", "docs"))
		return s, nil
	}
	out, err := run(t, scripted, "mkdir", "/docs")
	if err != nil || out != "mkdir: /docs (id A)\n" {
		t.Errorf("mkdir = %q, %v", out, err)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), affected(), directory("A", "root", "docs"), affected())
		return s, nil
	}
	out, err = run(t, scripted, "mkdir", "/docs", "--unit", strings.ToUpper(unit))
	if err != nil || out != "mkdir: /docs (id A, unit "+unit+")\n" {
		t.Errorf("mkdir --unit = %q, %v; want the unit in canonical form", out, err)
	}
}

func TestParseSort(t *testing.T) {
	for in, want := range map[string]files.Sort{
		"name":            {Field: "name"},
		"name:asc":        {Field: "name"},
		"name:desc":       {Field: "name", Descending: true},
		"created_at:desc": {Field: "created_at", Descending: true},
	} {
		got, err := files.ParseSort(in)
		if err != nil || got != want {
			t.Errorf("ParseSort(%q) = %+v, %v, want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", ":desc", "name:", "name:down", "name:DESC"} {
		if _, err := files.ParseSort(in); err == nil {
			t.Errorf("ParseSort(%q) succeeded, want an error", in)
		}
	}
}

// TestCommands_RenderTheCursor proves ls prints a next-files: line when
// the file half has a next page, that the cursor it prints is accepted
// back as --after-files, that the half read after it says so and carries
// no total while the other half is still read by number with its total,
// and that --after-dirs continues the directory half the same way.
func TestCommands_RenderTheCursor(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 1, "docs"), listing(fileColumns, true, 3, "a.txt", "b.txt", "c.txt"))
		return s, nil
	}
	out, err := run(t, scripted, "ls", "/", "--size", "2")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(out, "next-dirs:") || !strings.Contains(out, "file  b.txt") || strings.Contains(out, "file  c.txt") {
		t.Errorf("page 1 rendered:\n%s", out)
	}
	var cursor string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "next-files: "); ok {
			cursor = rest
		}
	}
	if cursor == "" {
		t.Fatalf("page 1 printed no next-files line:\n%s", out)
	}

	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 1, "docs"), listing(fileColumns, false, 0, "c.txt"))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--after-files", cursor)
	if err != nil {
		t.Fatalf("ls --after-files: %v", err)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 2, total 1\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\n") || strings.Contains(out, "next-") {
		t.Errorf("the cursor page rendered:\n%s", out)
	}

	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0, "a", "b", "c"), listing(fileColumns, true, 0))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(out, "next-dirs: ") || strings.Contains(out, "next-files:") {
		t.Errorf("a directory half with a next page rendered:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "next-dirs: "); ok {
			cursor = rest
		}
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, false, 0, "c"), listing(fileColumns, true, 0))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--after-dirs", cursor)
	if err != nil {
		t.Fatalf("ls --after-dirs: %v", err)
	}
	if !strings.Contains(out, "directories: 1 after the cursor, size 2, total not counted\n") || !strings.Contains(out, "files: 0 on page 1 of size 2, total 0\n") {
		t.Errorf("the directory cursor page rendered:\n%s", out)
	}

	// A cursor the listing did not issue is refused with its reason.
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0))
		return s, nil
	}
	_, err = run(t, scripted, "ls", "/", "--after-files", "nonsense")
	if err == nil || !strings.Contains(err.Error(), "not one this listing issued") {
		t.Errorf("ls --after-files nonsense = %v, want the cursor refusal", err)
	}
}
