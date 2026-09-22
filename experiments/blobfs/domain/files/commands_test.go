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
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
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

// dirID and fileID are the ids the id:<uuid> forms name in these tests;
// the scripted rows carry them where a test reads a row back.
const (
	dirID  = "0193b0a2-2222-7000-8000-000000000002"
	fileID = "0193b0a2-3333-7000-8000-000000000003"
)

// ran returns the recorder's ops after Verify's prepares, and executed
// the calls after them, since a command verifies the store before its
// first use and the tests read what the command then ran.
func ran(rec *sqltest.Recorder) (string, []sqltest.Call) {
	var out []string
	var calls []sqltest.Call
	for _, c := range rec.Calls() {
		if c.Op == sqltest.OpPrepare {
			continue
		}
		out = append(out, string(c.Op))
		calls = append(calls, c)
	}
	return strings.Join(out, " "), calls
}

// fieldOf returns the value of one label of a stat record, or "" when the
// record has no such line.
func fieldOf(out, label string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if rest, ok := strings.CutPrefix(line, label+":"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func TestCommands_MountsMkdirAndLs(t *testing.T) {
	cmds := files.Commands(nil, nil)
	var names []string
	for _, c := range cmds {
		names = append(names, c.Name())
	}
	slices.Sort(names)
	if got := strings.Join(names, ","); got != "bookmark,cat,cp,ls,mkdir,mv,put,rm,rmdir,stat" {
		t.Errorf("Commands() = %s, want bookmark,cat,cp,ls,mkdir,mv,put,rm,rmdir,stat", got)
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
		{[]string{"cp", "/a.txt"}, `accepts 2 arg`},
		{[]string{"cp", "/a.txt", "/b.txt", "--fail-after", "complete"}, `the step is insert or write`},
		{[]string{"rm"}, `accepts 1 arg`},
		{[]string{"rm", "/a.txt", "--fail-after", "complete"}, `the step is begin or object`},
		{[]string{"rm", "-r", "/a", "--fail-after", "begin"}, `rm -r takes no step`},
		{[]string{"rmdir", "/a", "/b"}, `accepts 1 arg`},
		{[]string{"bookmark", "add", "/a.txt"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "add", "/a.txt", "--unit", "nope"}, `--unit "nope" is not a UUID`},
		{[]string{"bookmark", "add", "--unit", unit}, `accepts 1 arg`},
		{[]string{"bookmark", "ls"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "ls", "--unit", unit, "--total", "some"}, `the mode is exact or none`},
		{[]string{"bookmark", "ls", "--unit", unit, "--sort", "path:up"}, `the direction is asc or desc`},
		{[]string{"bookmark", "ls", "/a", "--unit", unit}, `unknown command "/a"`},
		{[]string{"bookmark", "rm", "/a.txt"}, `required flag(s) "unit" not set`},
		{[]string{"bookmark", "rm", "--unit", unit}, `accepts 1 arg`},
		{[]string{"ls", "id:nope"}, `invalid id "nope": must be a UUID`},
		{[]string{"ls", "id:"}, `must be a UUID`},
		{[]string{"ls", "id:00000000-0000-0000-0000-000000000000"}, `the nil UUID is the root's`},
		{[]string{"ls", "id:" + dirID, "--unit", unit}, `list the path instead`},
		{[]string{"ls", "/", "--filter", "name"}, `write <field>:<op>:<value>`},
		{[]string{"ls", "/", "--filter", ":eq:x"}, `write <field>:<op>:<value>`},
		{[]string{"ls", "/", "--filter", "name::x"}, `names no operator`},
		{[]string{"ls", "/", "--filter", "name:eq"}, `names no value`},
		{[]string{"ls", "/", "--filter", "etag:null:x"}, `null takes no value`},
		{[]string{"ls", "/", "--filter", "status:in"}, `in takes a comma-separated list`},
		{[]string{"cat", "id:bad"}, `must be a UUID`},
		{[]string{"stat", "id:bad"}, `must be a UUID`},
		{[]string{"rm", "id:bad"}, `must be a UUID`},
		{[]string{"rm", "-r", "id:" + dirID}, `removed by path, not by id`},
		{[]string{"put", "-", "id:" + dirID}, `stdin has no name`},
		{[]string{"put", "-", "id:bad"}, `must be a UUID`},
		{[]string{"cp", "/a.txt", "id:" + dirID}, `two paths, or two ids`},
		{[]string{"cp", "id:" + fileID, "/b"}, `two paths, or two ids`},
		{[]string{"cp", "id:bad", "id:" + dirID}, `must be a UUID`},
		{[]string{"mv", "id:" + fileID, "/b"}, `two paths, or two ids`},
		{[]string{"mv", "/a", "id:" + dirID}, `two paths, or two ids`},
		{[]string{"mv", "id:" + fileID, "id:bad"}, `must be a UUID`},
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
		{"cp", "/a.txt", "/b.txt"},
		{"cp", "/a.txt", "/b.txt", "--fail-after", "write"},
		{"rm", "/a.txt"},
		{"rm", "-r", "/a"},
		{"rm", "/a.txt", "--fail-after", "object"},
		{"rmdir", "/a"},
		{"bookmark", "add", "/a.txt", "--unit", unit, "--active"},
		{"bookmark", "ls", "--unit", unit, "--page", "2", "--size", "5", "--sort", "path:desc", "--total", "none"},
		{"bookmark", "rm", "/a.txt", "--unit", unit},
		{"ls", "id:" + dirID, "--filter", "name:like:a%", "--cursors"},
		{"cat", "id:" + fileID},
		{"stat", "id:" + fileID},
		{"rm", "id:" + fileID, "--fail-after", "begin"},
		{"cp", "id:" + fileID, "id:" + dirID},
		{"mv", "id:" + fileID, "id:" + dirID},
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
// with a line per half and a more: line under each, and that the flags
// reach the store: page, size, and the total mode. A half whose sort a
// cursor cannot continue still says more: yes when rows remain, with no
// next-files: line, so the reader pages by number.
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
	if len(lines) != 8 || !strings.HasPrefix(lines[0], "KIND") || !strings.HasSuffix(lines[0], "  ID") || !strings.HasPrefix(lines[1], "dir   docs") || !strings.HasPrefix(lines[2], "file  a.txt  1") || !strings.HasPrefix(lines[3], "file  b.txt  2") {
		t.Errorf("stdout =\n%s", out)
	}
	if !strings.HasSuffix(lines[1], "  id-docs") || !strings.HasSuffix(lines[2], "  id-a.txt") || !strings.HasSuffix(lines[3], "  id-b.txt") {
		t.Errorf("the id column is not last:\n%s", out)
	}
	if lines[4] != "directories: 1 on page 2 of size 2, total 3" || lines[5] != "more: no" || lines[6] != "files: 2 on page 2 of size 2, total 9" || lines[7] != "more: no" {
		t.Errorf("half lines = %q, %q, %q, %q", lines[4], lines[5], lines[6], lines[7])
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
	if !strings.Contains(out, "directories: 0 on page 3 of size 20, total unknown (the page is empty)\nmore: no\n") || !strings.Contains(out, "files: 0 on page 3 of size 20, total unknown (the page is empty)\nmore: no\n") {
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
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total not counted\nmore: no\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total not counted\nmore: no\n") {
		t.Errorf("--total none rendered:\n%s", out)
	}

	// A sort by size cannot be continued by a cursor: the file half says
	// more: yes and prints no cursor.
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0), listing(fileColumns, true, 3, "a.txt", "b.txt", "c.txt"))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--sort", "size")
	if err != nil {
		t.Fatalf("ls --sort size: %v", err)
	}
	if !strings.Contains(out, "files: 2 on page 1 of size 2, total 3\nmore: yes\n") || strings.Contains(out, "next-") || strings.Contains(out, "file  c.txt") {
		t.Errorf("a half by a sort a cursor cannot continue rendered:\n%s", out)
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
// line, with - for a size or etag the row lacks; that a path no file is
// at falls through to the directory there, and / to the root, printed as
// the directory's fields; that a path neither kind holds is the file's
// not-found; and that an id is looked up the file first and then the
// directory, with no path line.
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

	// A directory by path: the file lookup finds nothing, and the path is
	// resolved as a directory.
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), noFile(), root(), directory(dirID, blobfs.RootID, "docs"))
		return s, nil
	}
	out, err = run(t, scripted, "stat", "/docs")
	if err != nil {
		t.Fatalf("stat of a directory: %v", err)
	}
	if fieldOf(out, "path") != "/docs" || fieldOf(out, "id") != dirID || fieldOf(out, "parent") != blobfs.RootID || fieldOf(out, "name") != "docs" || fieldOf(out, "version") != "1" || strings.Contains(out, "status:") {
		t.Errorf("stat of a directory rendered:\n%s", out)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root())
		return s, nil
	}
	out, err = run(t, scripted, "stat", "/")
	if err != nil || fieldOf(out, "path") != "/" || fieldOf(out, "id") != blobfs.RootID || fieldOf(out, "parent") != "-" || fieldOf(out, "name") != "/" {
		t.Errorf("stat / = %q, %v", out, err)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), noFile(), root(), noDirectory())
		return s, nil
	}
	out, err = run(t, scripted, "stat", "/missing")
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), "stat /missing") || out != "" {
		t.Errorf("stat of a path neither kind holds = %q, %v; want the file's not-found", out, err)
	}

	// By id: the file first, then the directory, and no path line.
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, file(fileID, "a.txt", "available", 2))
		return s, nil
	}
	out, err = run(t, scripted, "stat", "id:"+fileID)
	if err != nil || fieldOf(out, "id") != fileID || fieldOf(out, "status") != "available" || strings.Contains(out, "path:") {
		t.Errorf("stat by a file's id = %q, %v", out, err)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, noFile(), directory(dirID, blobfs.RootID, "docs"))
		return s, nil
	}
	out, err = run(t, scripted, "stat", "id:"+dirID)
	if err != nil || fieldOf(out, "id") != dirID || fieldOf(out, "name") != "docs" || fieldOf(out, "parent") != blobfs.RootID || strings.Contains(out, "path:") || strings.Contains(out, "status:") {
		t.Errorf("stat by a directory's id = %q, %v", out, err)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, noFile(), noDirectory())
		return s, nil
	}
	out, err = run(t, scripted, "stat", "id:"+dirID)
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), "no file or directory has it") || out != "" {
		t.Errorf("stat by an id neither kind holds = %q, %v", out, err)
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

// TestCommands_RenderTheCursor proves ls prints no cursor line by default
// and, under --cursors, a next-files: line under its more: yes line when
// the file half has a next page, that the cursor it prints is accepted
// back as --after-files, that the half read after it says so, carries no
// total, and says more: no on the last page while the other half is
// still read by number with its total, and that --after-dirs continues
// the directory half the same way.
func TestCommands_RenderTheCursor(t *testing.T) {
	scripted := func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 1, "docs"), listing(fileColumns, true, 3, "a.txt", "b.txt", "c.txt"))
		return s, nil
	}
	out, err := run(t, scripted, "ls", "/", "--size", "2")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(out, "next-") || !strings.Contains(out, "files: 2 on page 1 of size 2, total 3\nmore: yes\n") {
		t.Errorf("page 1 without --cursors rendered:\n%s", out)
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--cursors")
	if err != nil {
		t.Fatalf("ls --cursors: %v", err)
	}
	if strings.Contains(out, "next-dirs:") || !strings.Contains(out, "file  b.txt") || strings.Contains(out, "file  c.txt") {
		t.Errorf("page 1 rendered:\n%s", out)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 2, total 1\nmore: no\n") || !strings.Contains(out, "files: 2 on page 1 of size 2, total 3\nmore: yes\nnext-files: ") {
		t.Errorf("page 1's more: lines rendered:\n%s", out)
	}
	var cursor string
	for line := range strings.SplitSeq(out, "\n") {
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
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--after-files", cursor, "--cursors")
	if err != nil {
		t.Fatalf("ls --after-files: %v", err)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 2, total 1\nmore: no\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\nmore: no\n") || strings.Contains(out, "next-") {
		t.Errorf("the cursor page rendered:\n%s", out)
	}

	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0, "a", "b", "c"), listing(fileColumns, true, 0))
		return s, nil
	}
	out, err = run(t, scripted, "ls", "/", "--size", "2", "--cursors")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(out, "more: yes\nnext-dirs: ") || strings.Contains(out, "next-files:") {
		t.Errorf("a directory half with a next page rendered:\n%s", out)
	}
	for line := range strings.SplitSeq(out, "\n") {
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
	if !strings.Contains(out, "directories: 1 after the cursor, size 2, total not counted\nmore: no\n") || !strings.Contains(out, "files: 0 on page 1 of size 2, total 0\nmore: no\n") {
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
		s, _ := newStore(t, root(), file("F", "a.txt", "available", 2), affected(), affected())
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
		resp.Rows[1][3] = true // active
		s, _ := newStore(t, counted(7), resp)
		return s, nil
	}
	out, err = run(t, scripted, "bookmark", "ls", "--unit", unit, "--page", "2", "--size", "2")
	if err != nil {
		t.Fatalf("bookmark ls: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "PATH") || !strings.HasPrefix(lines[1], "/reports/2026/plan.txt  1     available  -") || !strings.HasPrefix(lines[2], "/notes.md               2     available  active") {
		t.Errorf("stdout =\n%s", out)
	}
	if lines[3] != "bookmarks: 2 on page 2 of size 2, total 7" || lines[4] != "more: yes" {
		t.Errorf("the page lines = %q, %q (page 2 of size 2 ends at row 4 of 7)", lines[3], lines[4])
	}
	// The read model's count runs under --total none too, so more: is
	// still derived from it: page 1 of size 20 holds 2 of 7.
	out, err = run(t, scripted, "bookmark", "ls", "--unit", unit, "--total", "none")
	if err != nil || !strings.HasSuffix(out, "bookmarks: 2 on page 1 of size 20, total not counted\nmore: yes\n") {
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
		s, _ := newStore(t, root(), file("F", "a.txt", "available", 2), affected(), violation(files.ConstraintUniqueBookmarkActive, sqlate.ErrUniqueViolation))
		return s, nil
	}
	out, err = run(t, scripted, "bookmark", "add", "/a.txt", "--unit", unit, "--active")
	if !errors.Is(err, files.ErrActiveBookmark) || out != "" {
		t.Errorf("a refused add = %q, %v; want ErrActiveBookmark and no result line", out, err)
	}
}

// TestCommands_ListByID proves ls id:<uuid> lists through ListDirectory:
// the directory is read by id, no path is resolved, and both halves
// print with their ids.
func TestCommands_ListByID(t *testing.T) {
	var rec *sqltest.Recorder
	scripted := func() (*files.Store, error) {
		var s *files.Store
		s, rec = newStore(t, directory(dirID, blobfs.RootID, "d"), listing(directoryColumns, true, 1, "x"), listing(fileColumns, true, 1, "a.txt"))
		return s, nil
	}
	out, err := run(t, scripted, "ls", "id:"+strings.ToUpper(dirID))
	if err != nil {
		t.Fatalf("ls by id: %v", err)
	}
	if got, calls := ran(rec); got != "begin query query query commit" || calls[1].Args[0] != dirID {
		t.Errorf("ops = %q and the first read bound %v; want the directory read by its canonical id, then the halves", got, calls[1].Args)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 7 || !strings.HasPrefix(lines[1], "dir   x ") || !strings.HasSuffix(lines[1], "  id-x") || !strings.HasPrefix(lines[2], "file  a.txt ") || !strings.HasSuffix(lines[2], "  id-a.txt") {
		t.Errorf("ls by id rendered:\n%s", out)
	}
	if lines[3] != "directories: 1 on page 1 of size 20, total 1" || lines[5] != "files: 1 on page 1 of size 20, total 1" {
		t.Errorf("the half lines = %q, %q", lines[3], lines[5])
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, noDirectory())
		return s, nil
	}
	if _, err := run(t, scripted, "ls", "id:"+dirID); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("ls by an id no directory holds = %v, want ErrNotFound", err)
	}
}

// TestCommands_FilterTheListing proves --filter reaches both halves as
// database.go lowers it: a filter on a shared field predicates both, one
// on a file-only field predicates the files alone, and the values are
// bound, not spliced.
func TestCommands_FilterTheListing(t *testing.T) {
	var rec *sqltest.Recorder
	scripted := func() (*files.Store, error) {
		var s *files.Store
		s, rec = newStore(t, root(), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
		return s, nil
	}
	if _, err := run(t, scripted, "ls", "/", "--filter", "name:like:a%", "--filter", "size:gt:10"); err != nil {
		t.Fatalf("ls --filter: %v", err)
	}
	queries := rec.SQL(sqltest.OpQuery)
	if !strings.Contains(queries[1], " AND q.name LIKE CAST($2 AS text) ORDER BY") || strings.Contains(queries[1], "size") {
		t.Errorf("the directory half took a file-only filter or lost the shared one:\n%s", queries[1])
	}
	if !strings.Contains(queries[2], " AND q.name LIKE CAST($2 AS text) AND q.size > CAST($3 AS bigint) ORDER BY") {
		t.Errorf("the file half did not take both filters:\n%s", queries[2])
	}
	if _, calls := ran(rec); len(calls[3].Args) != 5 || calls[3].Args[1] != "a%" || calls[3].Args[2] != "10" {
		args := calls[3].Args
		t.Errorf("the file half bound %v, want the directory, the two values, the offset, and the fetch", args)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, root(), listing(directoryColumns, true, 0))
		return s, nil
	}
	if _, err := run(t, scripted, "ls", "/", "--filter", "owner:eq:x"); !errors.Is(err, query.ErrDirectives) || !strings.Contains(err.Error(), "owner") {
		t.Errorf("ls --filter on a field neither half has = %v, want ErrDirectives naming it", err)
	}
}

// TestCommands_RenderTheIDForms proves each id form calls the id-keyed
// method with no resolution and renders the same line as the path form,
// naming the argument as given: cat streams the file's object, rm runs
// the three steps, put lands the local file under its base name in the
// directory, cp copies into the directory under the source's name, and
// mv moves a file, or else a directory, into the directory.
func TestCommands_RenderTheIDForms(t *testing.T) {
	opens := 0
	var rec *sqltest.Recorder
	var fake *storagetest.Fake
	scripted := func() (*files.Store, error) {
		var s *files.Store
		s, rec, fake = writeStore(t, &opens, file(fileID, "note.txt", "available", 2))
		if _, err := fake.Put(context.Background(), fileID+"/note.txt", strings.NewReader("by id"), storage.PutOptions{}); err != nil {
			return nil, err
		}
		return s, nil
	}
	out, err := run(t, scripted, "cat", "id:"+fileID)
	if err != nil || out != "by id" {
		t.Errorf("cat by id = %q, %v", out, err)
	}
	if got, calls := ran(rec); got != "query" || calls[0].Args[0] != fileID {
		t.Errorf("cat by id ran %q binding %v, want one read by id", got, calls[0].Args)
	}

	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, rec, fake = writeStore(t, &opens, affected(), file(fileID, "note.txt", "deleting", 2), bookmarkTotal(0), affected())
		stored(t, fake, fileID+"/note.txt")
		return s, nil
	}
	out, err = run(t, scripted, "rm", "id:"+fileID)
	if err != nil || out != "rm: id:"+fileID+" (id "+fileID+")\n" {
		t.Errorf("rm by id = %q, %v", out, err)
	}
	if got, _ := ran(rec); got != "begin exec query query commit exec" || held(t, fake, fileID+"/note.txt") {
		t.Errorf("rm by id ran %q, object held %v; want the three steps with no read first", got, held(t, fake, fileID+"/note.txt"))
	}

	local := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, rec, fake = writeStore(t, &opens, noFile(), affected(), fileIn(fileID, dirID, "note.txt", "pending", 1), affected(), fileIn(fileID, dirID, "note.txt", "available", 2))
		return s, nil
	}
	out, err = run(t, scripted, "put", local, "id:"+dirID)
	if err != nil || !strings.HasPrefix(out, "put: note.txt in id:"+dirID+" (id "+fileID+", 5 bytes, etag \"") {
		t.Errorf("put into a directory by id = %q, %v", out, err)
	}
	if got, calls := ran(rec); got != "begin query exec query commit exec query" || calls[1].Args[0] != dirID || calls[1].Args[1] != "note.txt" {
		t.Errorf("put by id ran %q, the lookup binding %v; want no resolution and the local file's base name", got, calls[1].Args)
	}
	if opts, _ := fake.LastPut(); !strings.HasPrefix(opts.ContentType, "text/plain") {
		t.Errorf("put by id declared %+v, want the type of the local file's extension", opts)
	}

	const copyID = "0193b0a2-4444-7000-8000-000000000004"
	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, rec, fake = writeStore(t, &opens, file(fileID, "note.txt", "available", 2), noFile(), affected(), fileIn(copyID, dirID, "note.txt", "pending", 1), affected(), fileIn(copyID, dirID, "note.txt", "available", 2))
		if _, err := fake.Put(context.Background(), fileID+"/note.txt", strings.NewReader("hello"), storage.PutOptions{ContentType: "text/plain"}); err != nil {
			return nil, err
		}
		return s, nil
	}
	out, err = run(t, scripted, "cp", "id:"+fileID, "id:"+dirID)
	if err != nil || !strings.HasPrefix(out, "cp: id:"+fileID+" -> id:"+dirID+" (id "+copyID+", 5 bytes, etag \"") {
		t.Errorf("cp by ids = %q, %v", out, err)
	}
	if got, calls := ran(rec); got != "begin query query exec query commit exec query" || calls[2].Args[0] != dirID || calls[2].Args[1] != "note.txt" {
		t.Errorf("cp by ids ran %q, the lookup binding %v; want the source read by id and the destination by id under the source's name", got, calls[2].Args)
	}

	// mv: the source is read once on the pool to learn its kind, then
	// MoveEntry runs in its transaction.
	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, rec = newStore(t, fileIn(fileID, "S", "a.txt", "available", 1), fileIn(fileID, "S", "a.txt", "available", 1), ancestors("s"), ancestors("s", "t"), affected(), fileIn(fileID, dirID, "a.txt", "available", 2))
		return s, nil
	}
	out, err = run(t, scripted, "mv", "id:"+fileID, "id:"+dirID)
	if err != nil || out != "mv: /s/a.txt -> /s/t/a.txt (id "+fileID+")\n" {
		t.Errorf("mv of a file by ids = %q, %v", out, err)
	}
	if got, calls := ran(rec); got != "query begin query query query exec query commit" || calls[5].Args[0] != dirID {
		t.Errorf("mv by ids ran %q, the update binding %v; want the kind read on the pool, then the move into the directory", got, calls[5].Args)
	}
	scripted = func() (*files.Store, error) {
		var s *files.Store
		s, rec = newStore(t, noFile(), directory("X", "A", "x"), directory("X", "A", "x"), ancestors("a"), ancestors("a", "y"), within(0), affected(), directory("X", dirID, "x"))
		return s, nil
	}
	out, err = run(t, scripted, "mv", "id:"+fileID, "id:"+dirID)
	if err != nil || out != "mv: /a/x -> /a/y/x (id "+fileID+")\n" {
		t.Errorf("mv of a directory by ids = %q, %v", out, err)
	}
	if got, _ := ran(rec); got != "query query begin query query query query exec query commit" {
		t.Errorf("mv of a directory by ids ran %q; want the file miss and the directory read on the pool, then the move under the lock", got)
	}
	scripted = func() (*files.Store, error) {
		s, _ := newStore(t, noFile(), noDirectory())
		return s, nil
	}
	if _, err := run(t, scripted, "mv", "id:"+fileID, "id:"+dirID); !errors.Is(err, blobfs.ErrNotFound) || !strings.Contains(err.Error(), "no file or directory has it") {
		t.Errorf("mv of an id neither kind holds = %v", err)
	}
}

func TestParseRef(t *testing.T) {
	for in, want := range map[string]files.Ref{
		"/":                            {Path: "/"},
		"/a/b.txt":                     {Path: "/a/b.txt"},
		"relative":                     {Path: "relative"},
		"id:" + dirID:                  {ID: dirID},
		"id:" + strings.ToUpper(dirID): {ID: dirID},
		"id:urn:uuid:" + dirID:         {ID: dirID},
	} {
		got, err := files.ParseRef(in)
		if err != nil || got != want {
			t.Errorf("ParseRef(%q) = %+v, %v, want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"id:", "id:nope", "id:/a", "id:00000000-0000-0000-0000-000000000000", "id:" + dirID + "/name"} {
		if _, err := files.ParseRef(in); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("ParseRef(%q) = %v, want ErrInvalidID", in, err)
		}
	}
}

func TestParseFilter(t *testing.T) {
	for in, want := range map[string]files.Filter{
		"name:eq:docs":                       {Field: "name", Op: "eq", Value: "docs"},
		"name:like:a%":                       {Field: "name", Op: "like", Value: "a%"},
		"size:gt:10":                         {Field: "size", Op: "gt", Value: "10"},
		"created_at:ge:2026-01-01T00:00:00Z": {Field: "created_at", Op: "ge", Value: "2026-01-01T00:00:00Z"},
		"name:eq:":                           {Field: "name", Op: "eq", Value: ""},
		"etag:null":                          {Field: "etag", Op: "null"},
		"size:notnull":                       {Field: "size", Op: "notnull"},
		"name:between:x":                     {Field: "name", Op: "between", Value: "x"},
	} {
		got, err := files.ParseFilter(in)
		if err != nil || got != want {
			t.Errorf("ParseFilter(%q) = %+v, %v, want %+v", in, got, err, want)
		}
	}
	got, err := files.ParseFilter("status:in:available,pending")
	if err != nil || got.Field != "status" || got.Op != "in" || !slices.Equal(got.Value.([]any), []any{"available", "pending"}) {
		t.Errorf("ParseFilter of an in term = %+v, %v", got, err)
	}
	for _, in := range []string{"", "name", ":eq:x", "name::x", "name:eq", "etag:null:x", "status:in"} {
		if _, err := files.ParseFilter(in); err == nil {
			t.Errorf("ParseFilter(%q) succeeded, want an error", in)
		}
	}
}
