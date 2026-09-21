package files_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate"

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
	if got := strings.Join(names, ","); got != "bookmark,cat,ls,mkdir,put,stat" {
		t.Errorf("Commands() = %s, want bookmark,cat,ls,mkdir,put,stat", got)
	}
	for _, c := range cmds {
		if c.Name() != "bookmark" {
			continue
		}
		var subs []string
		for _, s := range c.Commands() {
			subs = append(subs, s.Name())
		}
		slices.Sort(subs)
		if got := strings.Join(subs, ","); got != "add,ls,rm" {
			t.Errorf("bookmark's subcommands = %s, want add,ls,rm", got)
		}
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
		{[]string{"put", "-", "/a.txt", "--fail-after", "complete"}, `the step is insert or write`},
		{[]string{"put", "/no/such/local/file", "/a.txt"}, `no such file`},
		{[]string{"put", "/a.txt"}, `accepts 2 arg`},
		{[]string{"cat"}, `accepts 1 arg`},
		{[]string{"stat", "/a", "/b"}, `accepts 1 arg`},
		{[]string{"bookmark", "add", "/a.txt"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "add", "/a.txt", "--unit", "nope"}, `--unit "nope" is not a UUID`},
		{[]string{"bookmark", "add", "--unit", unit}, `accepts 1 arg`},
		{[]string{"bookmark", "ls"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "ls", "--unit", unit, "--total", "some"}, `the mode is exact or none`},
		{[]string{"bookmark", "ls", "--unit", unit, "--sort", "path:up"}, `the direction is asc or desc`},
		{[]string{"bookmark", "ls", "/a", "--unit", unit}, `unknown command "/a"`},
		{[]string{"bookmark", "rm", "/a.txt"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "rm", "--unit", unit}, `accepts 1 arg`},
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
		{"put", "-", "/a.txt"},
		{"cat", "/a.txt"},
		{"stat", "/a.txt"},
		{"bookmark", "add", "/a.txt", "--unit", unit, "--active"},
		{"bookmark", "ls", "--unit", unit, "--page", "2", "--size", "5", "--sort", "path:desc", "--total", "none"},
		{"bookmark", "rm", "/a.txt", "--unit", unit},
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

// TestCommands_RenderStat proves stat prints the row as one field per
// line, with - for a size or etag the row lacks.
func TestCommands_RenderStat(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), file("P", "a.txt", "pending", 1))
		return s, nil
	}
	out, err := run(t, scripted, "stat", "/a.txt")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	for _, want := range []string{"path:         /a.txt\n", "id:           P\n", "status:       pending\n", "size:         -\n", "etag:         -\n", "content-type: text/plain\n", "key:          P/a.txt\n", "version:      1\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("stat rendered:\n%s\nwant a line %q", out, want)
		}
	}
}

// TestCommands_RenderPutAndCat proves put reads its local file, declares
// the type its extension registers, prints one result line with the id,
// the size, and the etag, and says when it resumed a pending row; and
// that cat streams the object as it is.
func TestCommands_RenderPutAndCat(t *testing.T) {
	local := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	opens := 0
	var fake *storagetest.Fake
	scripted := func() (*files.Store, error) {
		var s *files.Store
		s, _, fake = writeStore(t, &opens, root(), noFile(), affected(), file("F", "note.txt", "pending", 1), affected(), file("F", "note.txt", "available", 2))
		return s, nil
	}
	out, err := run(t, scripted, "put", local, "/note.txt")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if !strings.HasPrefix(out, "put: /note.txt (id F, 5 bytes, etag \"") || strings.Contains(out, "resumed") {
		t.Errorf("put rendered %q", out)
	}
	if opts, n := fake.LastPut(); !strings.HasPrefix(opts.ContentType, "text/plain") || opts.Size != 5 || n != 5 {
		t.Errorf("put declared %+v and the store read %d bytes", opts, n)
	}

	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, _, fake = writeStore(t, &opens, root(), file("F", "note.txt", "pending", 1), affected(), file("F", "note.txt", "available", 2))
		return s, nil
	}
	out, err = run(t, scripted, "put", local, "/note.txt", "--content-type", "text/markdown")
	if err != nil || !strings.HasSuffix(out, ", resumed the pending row)\n") {
		t.Errorf("put over a pending row rendered %q, %v", out, err)
	}
	if opts, _ := fake.LastPut(); opts.ContentType != "text/markdown" {
		t.Errorf("put declared %+v, want the flag's type", opts)
	}

	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, _, fake = writeStore(t, &opens, root(), noFile(), affected(), file("F", "note.txt", "pending", 1))
		return s, nil
	}
	out, err = run(t, scripted, "put", local, "/note.txt", "--fail-after", "insert")
	if !errors.Is(err, files.ErrStopped) || out != "" {
		t.Errorf("put --fail-after insert = %q, %v; want the stop and no result line", out, err)
	}

	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, _, fake = writeStore(t, &opens, root(), file("A", "note.txt", "available", 2))
		if _, err := fake.Put(context.Background(), "A/note.txt", strings.NewReader("stored\x00bytes"), storage.PutOptions{}); err != nil {
			return nil, err
		}
		return s, nil
	}
	out, err = run(t, scripted, "cat", "/note.txt")
	if err != nil || out != "stored\x00bytes" {
		t.Errorf("cat = %q, %v", out, err)
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

// TestCommands_RenderBookmarks proves the bookmark commands render: add
// prints one result line naming the path, the file, the unit in canonical
// form, and whether the bookmark is active; ls prints the entries as
// aligned columns with the active marker and one line stating the page
// and the total, or its absence under --total none; rm prints one result
// line; and a refused add returns the sentinel's message unrendered.
func TestCommands_RenderBookmarks(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), file("F", "a.txt", "available", 2), affected())
		return s, nil
	}
	out, err := run(t, scripted, "bookmark", "add", "/a.txt", "--unit", strings.ToUpper(unit), "--active")
	if err != nil || out != "bookmark add: /a.txt (file F, unit "+unit+", active)\n" {
		t.Errorf("bookmark add --active = %q, %v", out, err)
	}
	out, err = run(t, scripted, "bookmark", "add", "/a.txt", "--unit", unit)
	if err != nil || out != "bookmark add: /a.txt (file F, unit "+unit+", inactive)\n" {
		t.Errorf("bookmark add = %q, %v", out, err)
	}

	scripted = func() (*files.Store, error) {
		resp := bookmarks(unit, "/reports/2026/plan.txt", "/notes.md")
		resp.Rows[1][2] = true
		s, _ := newStore(t, counted(7), resp)
		return s, nil
	}
	out, err = run(t, scripted, "bookmark", "ls", "--unit", unit, "--page", "2", "--size", "2")
	if err != nil {
		t.Fatalf("bookmark ls: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "PATH") || !strings.HasPrefix(lines[1], "/reports/2026/plan.txt  1     available  -") || !strings.HasPrefix(lines[2], "/notes.md               2     available  active") {
		t.Errorf("stdout =\n%s", out)
	}
	if lines[3] != "bookmarks: 2 on page 2 of size 2, total 7" {
		t.Errorf("the page line = %q", lines[3])
	}
	out, err = run(t, scripted, "bookmark", "ls", "--unit", unit, "--total", "none")
	if err != nil || !strings.HasSuffix(out, "bookmarks: 2 on page 1 of size 20, total not counted\n") {
		t.Errorf("bookmark ls --total none rendered %q, %v", out, err)
	}

	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), file("F", "a.txt", "available", 2), affected())
		return s, nil
	}
	out, err = run(t, scripted, "bookmark", "rm", "/a.txt", "--unit", unit)
	if err != nil || out != "bookmark rm: /a.txt (file F, unit "+unit+")\n" {
		t.Errorf("bookmark rm = %q, %v", out, err)
	}

	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), file("F", "a.txt", "available", 2), violation(files.ConstraintUniqueBookmarkActive, sqlate.ErrUniqueViolation))
		return s, nil
	}
	out, err = run(t, scripted, "bookmark", "add", "/a.txt", "--unit", unit, "--active")
	if !errors.Is(err, files.ErrActiveBookmark) || out != "" {
		t.Errorf("a refused add = %q, %v; want ErrActiveBookmark and no result line", out, err)
	}
}
