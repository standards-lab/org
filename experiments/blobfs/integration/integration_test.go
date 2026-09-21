//go:build integration

// Package integration drives the built binary black-box against the compose
// stack: it builds cmd/blobfs, runs it as a child process configured only
// through its environment and arguments, and checks the effect on a
// throwaway database. Every file carries the integration tag, so the
// package comment sits in this one.
package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// objectTables are the tables both migration sets create, in creation
// order.
var objectTables = []string{"blobfs_directory", "blobfs_file", "directory_owner", "bookmark"}

// build compiles cmd/blobfs into the test's temporary directory and returns
// the binary's path.
func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "blobfs")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/standards-lab/org/experiments/blobfs/cmd/blobfs")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// target is what one test's runs of the binary point at: the throwaway
// database, through BLOBFS_DSN, and the test's own container, through
// BLOBFS_STORAGE_CONTAINER; the other storage settings come from the
// process's environment, which mise sets. stdin, when not nil, is what
// the next run reads.
type target struct {
	dsn, container string
	stdin          io.Reader
}

// open creates the test's database and container and returns the target.
func open(t *testing.T) (*sqlate.DB, target) {
	t.Helper()
	db, dsn := livetest.OpenDSN(t)
	return db, target{dsn: dsn, container: livetest.Container(t)}
}

// run executes the binary with args against tg, and returns its stdout,
// stderr, and exit code.
func run(t *testing.T, bin string, tg target, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BLOBFS_DSN="+tg.dsn, "BLOBFS_STORAGE_CONTAINER="+tg.container)
	cmd.Stdin = tg.stdin
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), errOut.String(), code
}

// ok runs the binary and fails the test unless it exits zero, returning
// its stdout.
func ok(t *testing.T, bin string, tg target, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, bin, tg, args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

// refused runs the binary and fails the test unless it exits one with
// nothing on stdout and want on stderr.
func refused(t *testing.T, bin string, tg target, want string, args ...string) {
	t.Helper()
	out, errOut, code := run(t, bin, tg, args...)
	if code != 1 {
		t.Errorf("%v exited %d, want 1", args, code)
	}
	if out != "" {
		t.Errorf("%v: stdout = %q, want nothing", args, out)
	}
	if !strings.Contains(errOut, want) {
		t.Errorf("%v: stderr = %q, want %q", args, errOut, want)
	}
}

// lines splits stdout into its lines.
func lines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// column returns the second column of every entry line of a listing: the
// names, directories first, in the order printed.
func column(out string) []string {
	var names []string
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "dir ") || strings.HasPrefix(line, "file ") {
			names = append(names, strings.Fields(line)[1])
		}
	}
	return names
}

// insertFile inserts an available file row directly, since the write path
// is a later stage.
func insertFile(ctx context.Context, t *testing.T, db *sqlate.DB, dir, name string, size int64) {
	t.Helper()
	id := blobfs.NewID()
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size) VALUES ($1, $2, $3, 'available', $4, 'text/plain', $5)", id, dir, name, id+"/"+name, size); err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
}

// directoryID returns the id of the directory named name under the root.
func directoryID(ctx context.Context, t *testing.T, db *sqlate.DB, name string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT CAST(id AS text) FROM blobfs_directory WHERE parent_id = $1 AND name = $2", blobfs.RootID, name)
	if err != nil {
		t.Fatalf("directory %s: %v", name, err)
	}
	defer func() { _ = rows.Close() }()
	var id string
	if !rows.Next() || rows.Scan(&id) != nil {
		t.Fatalf("directory %s: no row: %v", name, rows.Err())
	}
	return id
}

// TestSchemaCommands runs the binary's schema up and schema down against a
// throwaway database named through BLOBFS_DSN alone: up creates the
// consumer's tables over blobfs's and seeds the root, prints its one result
// line, and exits zero; down removes every object table and exits zero.
func TestSchemaCommands(t *testing.T) {
	ctx := context.Background()
	db, tg := open(t)
	bin := build(t)

	out := ok(t, bin, tg, "schema", "up")
	if !strings.HasPrefix(out, "schema up:") {
		t.Errorf("schema up stdout = %q, want the result line", out)
	}
	for _, table := range objectTables {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after schema up, table %s is missing", table)
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id IS NULL")
	if err != nil {
		t.Fatalf("count roots: %v", err)
	}
	var roots int
	if !rows.Next() || rows.Scan(&roots) != nil {
		t.Fatalf("count roots: no row: %v", rows.Err())
	}
	_ = rows.Close()
	if roots != 1 {
		t.Errorf("after schema up, %d roots, want the one seeded root", roots)
	}

	out = ok(t, bin, tg, "schema", "down")
	if !strings.HasPrefix(out, "schema down:") {
		t.Errorf("schema down stdout = %q, want the result line", out)
	}
	for _, table := range objectTables {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after schema down, table %s still exists", table)
		}
	}
}

// TestFileCommands is the scripted run of the directory commands through
// the built binary: mkdir at nested paths and with --unit at a top-level
// path, the refusals mkdir renders, then ls with paging, sorting, --total
// none, the empty later page, and --unit scoping at a top-level directory
// and at the root. A listing against a database without the schema fails
// before any work and names schema up.
func TestFileCommands(t *testing.T) {
	ctx := context.Background()
	db, tg := open(t)
	bin := build(t)
	unit, other := blobfs.NewID(), blobfs.NewID()

	refused(t, bin, tg, "blobfs schema up", "ls", "/")
	ok(t, bin, tg, "schema", "up")

	// mkdir.
	if out := ok(t, bin, tg, "mkdir", "/reports"); !strings.HasPrefix(out, "mkdir: /reports (id ") {
		t.Errorf("mkdir stdout = %q", out)
	}
	ok(t, bin, tg, "mkdir", "/reports/2026")
	ok(t, bin, tg, "mkdir", "/reports/2025")
	if out := ok(t, bin, tg, "mkdir", "/archive", "--unit", unit); !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("mkdir --unit stdout = %q, want the unit named", out)
	}
	ok(t, bin, tg, "mkdir", "/archive/old")
	ok(t, bin, tg, "mkdir", "/theirs", "--unit", other)
	refused(t, bin, tg, "top-level directory only", "mkdir", "/reports/2024", "--unit", unit)
	refused(t, bin, tg, "name taken", "mkdir", "/reports")
	refused(t, bin, tg, "not found", "mkdir", "/missing/child")
	refused(t, bin, tg, "the root directory", "mkdir", "/")
	refused(t, bin, tg, "is not a UUID", "mkdir", "/x", "--unit", "nope")

	reports := directoryID(ctx, t, db, "reports")
	archive := directoryID(ctx, t, db, "archive")
	for i, name := range []string{"c.txt", "a.txt", "b.txt"} {
		insertFile(ctx, t, db, reports, name, int64(10*(3-i)))
	}
	insertFile(ctx, t, db, archive, "old.txt", 1)

	// ls: directories then files, one page each, with the totals.
	out := ok(t, bin, tg, "ls", "/reports")
	if got := column(out); strings.Join(got, " ") != "2025 2026 a.txt b.txt c.txt" {
		t.Errorf("ls /reports names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total 2\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total 3\n") {
		t.Errorf("ls /reports stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/reports", "--page", "2", "--size", "1", "--sort", "name:desc")
	if got := column(out); strings.Join(got, " ") != "2025 b.txt" {
		t.Errorf("ls page 2 of 1 by name desc names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 2 of size 1, total 2\n") || !strings.Contains(out, "files: 1 on page 2 of size 1, total 3\n") {
		t.Errorf("ls page 2 stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/reports", "--sort", "size:desc")
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt a.txt b.txt" {
		t.Errorf("ls by size desc names = %v; want directories in name order and files by size", got)
	}
	out = ok(t, bin, tg, "ls", "/reports", "--total", "none")
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total not counted\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total not counted\n") {
		t.Errorf("ls --total none stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/reports", "--page", "5")
	if !strings.Contains(out, "directories: 0 on page 5 of size 20, total unknown") || !strings.Contains(out, "files: 0 on page 5 of size 20, total unknown") {
		t.Errorf("an empty later page stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/reports/2026")
	if !strings.Contains(out, "directories: 0 on page 1 of size 20, total 0\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("an empty first page stdout:\n%s", out)
	}
	refused(t, bin, tg, "not found", "ls", "/reports/missing")
	refused(t, bin, tg, "unknown sort field", "ls", "/reports", "--sort", "owner")

	// The cursor: a half with a next page prints it, and the flag continues
	// that half alone, without a total.
	out = ok(t, bin, tg, "ls", "/reports", "--size", "2")
	if got := column(out); strings.Join(got, " ") != "2025 2026 a.txt b.txt" {
		t.Errorf("ls --size 2 names = %v", got)
	}
	cursor := ""
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, "next-files: "); found {
			cursor = rest
		}
		if strings.HasPrefix(line, "next-dirs:") {
			t.Errorf("the directory half printed a cursor with no next page:\n%s", out)
		}
	}
	if cursor == "" {
		t.Fatalf("ls --size 2 printed no next-files line:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/reports", "--size", "2", "--after-files", cursor)
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt" {
		t.Errorf("ls --after-files names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 2, total 2\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\n") || strings.Contains(out, "next-") {
		t.Errorf("ls --after-files stdout:\n%s", out)
	}
	refused(t, bin, tg, "not one this listing issued", "ls", "/reports", "--after-files", "nonsense")
	refused(t, bin, tg, "issued by files_in_directory", "ls", "/reports", "--after-dirs", cursor)
	refused(t, bin, tg, "size can be NULL", "ls", "/reports", "--sort", "size", "--after-files", cursor)

	// --unit at a top-level directory and below it.
	out = ok(t, bin, tg, "ls", "/archive", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "old old.txt" {
		t.Errorf("ls /archive as the owner names = %v", got)
	}
	ok(t, bin, tg, "ls", "/archive/old", "--unit", unit)
	refused(t, bin, tg, "does not own the directory", "ls", "/archive", "--unit", other)
	refused(t, bin, tg, "does not own the directory", "ls", "/archive/old", "--unit", other)
	refused(t, bin, tg, "does not own the directory", "ls", "/reports", "--unit", unit)

	// --unit at the root: the unit's own top-level directories.
	out = ok(t, bin, tg, "ls", "/", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "archive" {
		t.Errorf("ls / as the unit names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total 1\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("ls / as the unit stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "ls", "/", "--unit", other)
	if got := column(out); strings.Join(got, " ") != "theirs" {
		t.Errorf("ls / as the other unit names = %v", got)
	}
	refused(t, bin, tg, "owner read model takes no cursor", "ls", "/", "--unit", unit, "--after-dirs", cursor)
	out = ok(t, bin, tg, "ls", "/")
	if got := column(out); strings.Join(got, " ") != "archive reports theirs" {
		t.Errorf("ls / names = %v", got)
	}
}

// field returns the value of one label of a stat record.
func field(out, label string) string {
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, label+":"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// TestWriteCommands is the scripted run of the file commands through the
// built binary against Postgres and Azurite: put from a local file and
// from stdin, cat returning the bytes put stored, stat showing the row,
// the refusals put renders, and --fail-after at each step: the binary
// exits non-zero with the row left pending, which stat and ls show and
// cat refuses, and a put of the same path resumes and completes it. A
// name at blobfs's rune limit is accepted and one over is refused.
func TestWriteCommands(t *testing.T) {
	ctx := context.Background()
	db, tg := open(t)
	bin := build(t)
	ok(t, bin, tg, "schema", "up")
	ok(t, bin, tg, "mkdir", "/docs")
	local := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(local, []byte("quarterly\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// put, cat, stat.
	out := ok(t, bin, tg, "put", local, "/docs/report.txt")
	if !strings.HasPrefix(out, "put: /docs/report.txt (id ") || !strings.Contains(out, ", 10 bytes, etag \"") {
		t.Errorf("put stdout = %q", out)
	}
	if out := ok(t, bin, tg, "cat", "/docs/report.txt"); out != "quarterly\n" {
		t.Errorf("cat stdout = %q", out)
	}
	out = ok(t, bin, tg, "stat", "/docs/report.txt")
	if field(out, "status") != "available" || field(out, "size") != "10" || !strings.HasPrefix(field(out, "content-type"), "text/plain") ||
		!strings.HasPrefix(field(out, "etag"), "\"") || field(out, "version") != "2" || field(out, "name") != "report.txt" {
		t.Errorf("stat stdout:\n%s", out)
	}
	tg.stdin = strings.NewReader("from stdin")
	if out := ok(t, bin, tg, "put", "-", "/docs/notes.md", "--content-type", "text/markdown"); !strings.Contains(out, ", 10 bytes, etag ") {
		t.Errorf("put - stdout = %q", out)
	}
	tg.stdin = nil
	if out := ok(t, bin, tg, "cat", "/docs/notes.md"); out != "from stdin" {
		t.Errorf("cat of the stdin put = %q", out)
	}
	if out := ok(t, bin, tg, "stat", "/docs/notes.md"); field(out, "content-type") != "text/markdown" {
		t.Errorf("stat of the stdin put:\n%s", out)
	}
	if got := column(ok(t, bin, tg, "ls", "/docs")); strings.Join(got, " ") != "notes.md report.txt" {
		t.Errorf("ls /docs names = %v", got)
	}

	// Refusals.
	refused(t, bin, tg, "name taken", "put", local, "/docs/report.txt")
	refused(t, bin, tg, "not found", "put", local, "/missing/report.txt")
	refused(t, bin, tg, "not found", "cat", "/docs/missing.txt")
	refused(t, bin, tg, "not found", "stat", "/docs/missing.txt")
	refused(t, bin, tg, "the step is insert or write", "put", local, "/docs/x.txt", "--fail-after", "complete")
	refused(t, bin, tg, "the root directory", "put", local, "/")

	// --fail-after insert: the row is pending and visible, nothing is
	// stored, and the retry completes it.
	_, errOut, code := run(t, bin, tg, "put", local, "/docs/stopped.txt", "--fail-after", "insert")
	if code != 1 || !strings.Contains(errOut, "stopped after step insert") || !strings.Contains(errOut, "rerun put") {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}
	out = ok(t, bin, tg, "stat", "/docs/stopped.txt")
	if field(out, "status") != "pending" || field(out, "size") != "-" || field(out, "etag") != "-" || field(out, "version") != "1" {
		t.Errorf("stat after the stop:\n%s", out)
	}
	listed := false
	for _, line := range lines(ok(t, bin, tg, "ls", "/docs")) {
		if f := strings.Fields(line); len(f) > 3 && f[1] == "stopped.txt" {
			listed = f[2] == "-" && f[3] == "pending"
		}
	}
	if !listed {
		t.Errorf("ls after the stop does not show stopped.txt pending with no size")
	}
	refused(t, bin, tg, "the file is pending", "cat", "/docs/stopped.txt")
	rows, err := db.QueryContext(ctx, "SELECT status FROM blobfs_file WHERE name = 'stopped.txt'")
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if !rows.Next() || rows.Scan(&status) != nil || status != "pending" {
		t.Errorf("the database holds status %q for the stopped put", status)
	}
	_ = rows.Close()
	out = ok(t, bin, tg, "put", local, "/docs/stopped.txt")
	if !strings.HasSuffix(out, ", resumed the pending row)\n") {
		t.Errorf("the retry stdout = %q, want the resumed row", out)
	}
	if out := ok(t, bin, tg, "cat", "/docs/stopped.txt"); out != "quarterly\n" {
		t.Errorf("cat after the retry = %q", out)
	}
	if out := ok(t, bin, tg, "stat", "/docs/stopped.txt"); field(out, "status") != "available" || field(out, "version") != "2" {
		t.Errorf("stat after the retry:\n%s", out)
	}

	// --fail-after write: the object is stored, the row is still pending,
	// and the retry completes it.
	_, errOut, code = run(t, bin, tg, "put", local, "/docs/written.txt", "--fail-after", "write")
	if code != 1 || !strings.Contains(errOut, "stopped after step write") {
		t.Fatalf("put --fail-after write exited %d: %s", code, errOut)
	}
	if out := ok(t, bin, tg, "stat", "/docs/written.txt"); field(out, "status") != "pending" {
		t.Errorf("stat after the stop after write:\n%s", out)
	}
	if out := ok(t, bin, tg, "put", local, "/docs/written.txt"); !strings.Contains(out, "resumed the pending row") {
		t.Errorf("the retry after write = %q", out)
	}
	if out := ok(t, bin, tg, "cat", "/docs/written.txt"); out != "quarterly\n" {
		t.Errorf("cat after the retry = %q", out)
	}

	// The rune boundary of a name.
	name := strings.Repeat("\u00e9", blobfs.MaxNameLength)
	ok(t, bin, tg, "put", local, "/docs/"+name)
	if out := ok(t, bin, tg, "cat", "/docs/"+name); out != "quarterly\n" {
		t.Errorf("cat of the boundary name = %q", out)
	}
	refused(t, bin, tg, "at most 255 characters", "put", local, "/docs/"+name+"\u00e9")
}

// bookmarkPaths returns the first column of every entry line of a
// bookmark listing: the paths, in the order printed.
func bookmarkPaths(out string) []string {
	var paths []string
	for _, line := range lines(out) {
		if strings.HasPrefix(line, "/") {
			paths = append(paths, strings.Fields(line)[0])
		}
	}
	return paths
}

// TestBookmarkCommands is the scripted run of the bookmark commands
// through the built binary: add at depth six and at the root, the active
// bookmark and the refusal of a second one, the duplicate add, the
// missing file, ls with its full paths, the active marker, the pending
// file, paging, sorting, --total none, and the unit filter, then rm of
// the active bookmark and the refusals rm renders.
func TestBookmarkCommands(t *testing.T) {
	db, tg := open(t)
	bin := build(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	ok(t, bin, tg, "schema", "up")
	local := filepath.Join(t.TempDir(), "plan.txt")
	if err := os.WriteFile(local, []byte("plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := ""
	for _, name := range []string{"d1", "d2", "d3", "d4", "d5", "d6"} {
		dir += "/" + name
		ok(t, bin, tg, "mkdir", dir)
	}
	ok(t, bin, tg, "mkdir", "/reports")
	deep := dir + "/plan.txt"
	ok(t, bin, tg, "put", local, deep)
	ok(t, bin, tg, "put", local, "/root.txt")
	ok(t, bin, tg, "put", local, "/reports/x.txt")
	ok(t, bin, tg, "put", local, "/reports/y.txt")
	if _, errOut, code := run(t, bin, tg, "put", local, "/reports/draft.bin", "--fail-after", "insert"); code != 1 {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}
	_ = db

	// add: the result line, the active bookmark, and the refusals.
	out := ok(t, bin, tg, "bookmark", "add", deep, "--unit", unit)
	if !strings.HasPrefix(out, "bookmark add: "+deep+" (file ") || !strings.HasSuffix(out, ", unit "+unit+", inactive)\n") {
		t.Errorf("bookmark add stdout = %q", out)
	}
	if out := ok(t, bin, tg, "bookmark", "add", "/reports/x.txt", "--unit", unit, "--active"); !strings.HasSuffix(out, ", unit "+unit+", active)\n") {
		t.Errorf("bookmark add --active stdout = %q", out)
	}
	ok(t, bin, tg, "bookmark", "add", "/root.txt", "--unit", unit)
	ok(t, bin, tg, "bookmark", "add", "/reports/draft.bin", "--unit", unit)
	ok(t, bin, tg, "bookmark", "add", "/reports/x.txt", "--unit", other, "--active")
	refused(t, bin, tg, "has an active bookmark already", "bookmark", "add", "/reports/y.txt", "--unit", unit, "--active")
	refused(t, bin, tg, "has bookmarked the file already", "bookmark", "add", "/root.txt", "--unit", unit)
	refused(t, bin, tg, "not found", "bookmark", "add", "/reports/missing.txt", "--unit", unit)
	refused(t, bin, tg, "not found", "bookmark", "add", "/reports", "--unit", unit)
	refused(t, bin, tg, `required flag(s) "unit" not set`, "bookmark", "add", "/root.txt")
	refused(t, bin, tg, "is not a UUID", "bookmark", "add", "/root.txt", "--unit", "nope")

	// ls: full paths in path order, the active marker, the pending file
	// with its status, the total; then paging, sorting, --total none, and
	// the other unit's own listing.
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", unit)
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep+" /reports/draft.bin /reports/x.txt /root.txt" {
		t.Errorf("bookmark ls paths = %v", got)
	}
	if !strings.Contains(out, "bookmarks: 4 on page 1 of size 20, total 4\n") {
		t.Errorf("bookmark ls stdout:\n%s", out)
	}
	for _, line := range lines(out) {
		f := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "/reports/x.txt") && (f[2] != "available" || f[3] != "active"):
			t.Errorf("the active bookmark's line = %q", line)
		case strings.HasPrefix(line, "/reports/draft.bin") && (f[1] != "-" || f[2] != "pending" || f[3] != "-"):
			t.Errorf("the pending file's line = %q", line)
		case strings.HasPrefix(line, deep) && (f[1] != "5" || f[3] != "-"):
			t.Errorf("the deep file's line = %q", line)
		}
	}
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", unit, "--page", "2", "--size", "3", "--sort", "path:desc")
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep {
		t.Errorf("bookmark ls page 2 of 3 by path desc = %v", got)
	}
	if !strings.Contains(out, "bookmarks: 1 on page 2 of size 3, total 4\n") {
		t.Errorf("bookmark ls page 2 stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", unit, "--total", "none")
	if !strings.Contains(out, "bookmarks: 4 on page 1 of size 20, total not counted\n") {
		t.Errorf("bookmark ls --total none stdout:\n%s", out)
	}
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", other)
	if got := bookmarkPaths(out); strings.Join(got, " ") != "/reports/x.txt" {
		t.Errorf("bookmark ls of the other unit = %v", got)
	}
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", blobfs.NewID())
	if !strings.Contains(out, "bookmarks: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("bookmark ls of a unit with none:\n%s", out)
	}
	refused(t, bin, tg, "unknown sort field", "bookmark", "ls", "--unit", unit, "--sort", "key")
	refused(t, bin, tg, `required flag(s) "unit" not set`, "bookmark", "ls")

	// rm: the active bookmark goes, another can become active, and the
	// refusals.
	out = ok(t, bin, tg, "bookmark", "rm", "/reports/x.txt", "--unit", unit)
	if !strings.HasPrefix(out, "bookmark rm: /reports/x.txt (file ") || !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("bookmark rm stdout = %q", out)
	}
	ok(t, bin, tg, "bookmark", "add", "/reports/y.txt", "--unit", unit, "--active")
	refused(t, bin, tg, "has no bookmark of the file", "bookmark", "rm", "/reports/x.txt", "--unit", unit)
	refused(t, bin, tg, "not found", "bookmark", "rm", "/reports/missing.txt", "--unit", unit)
	out = ok(t, bin, tg, "bookmark", "ls", "--unit", unit)
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep+" /reports/draft.bin /reports/y.txt /root.txt" {
		t.Errorf("bookmark ls after rm = %v", got)
	}
	if got := bookmarkPaths(ok(t, bin, tg, "bookmark", "ls", "--unit", other)); strings.Join(got, " ") != "/reports/x.txt" {
		t.Errorf("the other unit's bookmark after the unit's rm = %v", got)
	}
}

// TestDeleteCommands is the scripted run of the delete commands through
// the built binary against Postgres and Azurite: rm --fail-after at each
// step, the deleting row that stat and ls show and cat and put refuse,
// the rm that finishes it, the bookmark that refuses an rm until it is
// removed, rm of an abandoned put, rmdir of an empty and of a non-empty
// directory and of an owned one, the root refusals, and rm -r over a
// tree with its per-entry lines and its summary.
func TestDeleteCommands(t *testing.T) {
	ctx := context.Background()
	db, tg := open(t)
	bin := build(t)
	unit := blobfs.NewID()
	ok(t, bin, tg, "schema", "up")
	local := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(local, []byte("note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok(t, bin, tg, "mkdir", "/docs")
	ok(t, bin, tg, "mkdir", "/docs/sub")
	ok(t, bin, tg, "put", local, "/docs/a.txt")
	ok(t, bin, tg, "put", local, "/docs/b.txt")
	ok(t, bin, tg, "put", local, "/docs/sub/c.txt")

	// --fail-after begin: the row is deleting and visible, and the retry
	// finishes the delete.
	_, errOut, code := run(t, bin, tg, "rm", "/docs/a.txt", "--fail-after", "begin")
	if code != 1 || !strings.Contains(errOut, "stopped after step begin") || !strings.Contains(errOut, "the row is deleting") || !strings.Contains(errOut, "rerun rm") {
		t.Fatalf("rm --fail-after begin exited %d: %s", code, errOut)
	}
	if out := ok(t, bin, tg, "stat", "/docs/a.txt"); field(out, "status") != "deleting" || field(out, "version") != "3" {
		t.Errorf("stat after the stop:\n%s", out)
	}
	listed := false
	for _, line := range lines(ok(t, bin, tg, "ls", "/docs")) {
		if f := strings.Fields(line); len(f) > 3 && f[1] == "a.txt" {
			listed = f[3] == "deleting"
		}
	}
	if !listed {
		t.Errorf("ls after the stop does not show a.txt deleting")
	}
	refused(t, bin, tg, "the file is deleting", "cat", "/docs/a.txt")
	refused(t, bin, tg, "name taken", "put", local, "/docs/a.txt")
	if out := ok(t, bin, tg, "rm", "/docs/a.txt"); !strings.HasPrefix(out, "rm: /docs/a.txt (id ") {
		t.Errorf("the finishing rm stdout = %q", out)
	}
	refused(t, bin, tg, "not found", "stat", "/docs/a.txt")
	refused(t, bin, tg, "not found", "rm", "/docs/a.txt")

	// --fail-after object: the object is gone, the row stays deleting, and
	// the retry removes it.
	_, errOut, code = run(t, bin, tg, "rm", "/docs/b.txt", "--fail-after", "object")
	if code != 1 || !strings.Contains(errOut, "stopped after step object") {
		t.Fatalf("rm --fail-after object exited %d: %s", code, errOut)
	}
	if out := ok(t, bin, tg, "stat", "/docs/b.txt"); field(out, "status") != "deleting" {
		t.Errorf("stat after the stop after object:\n%s", out)
	}
	ok(t, bin, tg, "rm", "/docs/b.txt")
	refused(t, bin, tg, "not found", "stat", "/docs/b.txt")
	rows, err := db.QueryContext(ctx, "SELECT COUNT(*) FROM blobfs_file WHERE name IN ('a.txt', 'b.txt')")
	if err != nil {
		t.Fatal(err)
	}
	var remaining int
	if !rows.Next() || rows.Scan(&remaining) != nil || remaining != 0 {
		t.Errorf("%d rows of the removed files remain", remaining)
	}
	_ = rows.Close()

	// A bookmark refuses the rm until it is removed.
	ok(t, bin, tg, "bookmark", "add", "/docs/sub/c.txt", "--unit", unit)
	refused(t, bin, tg, "the file is bookmarked", "rm", "/docs/sub/c.txt")
	if out := ok(t, bin, tg, "stat", "/docs/sub/c.txt"); field(out, "status") != "available" {
		t.Errorf("stat after the refused rm:\n%s", out)
	}
	ok(t, bin, tg, "bookmark", "rm", "/docs/sub/c.txt", "--unit", unit)
	ok(t, bin, tg, "rm", "/docs/sub/c.txt")

	// An abandoned put is removed the same way.
	if _, errOut, code := run(t, bin, tg, "put", local, "/docs/abandoned.txt", "--fail-after", "insert"); code != 1 {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}
	ok(t, bin, tg, "rm", "/docs/abandoned.txt")
	refused(t, bin, tg, "not found", "stat", "/docs/abandoned.txt")

	// rmdir: the refusals, then an empty directory and an owned one.
	refused(t, bin, tg, "not empty", "rmdir", "/docs")
	refused(t, bin, tg, "the root directory", "rmdir", "/")
	refused(t, bin, tg, "not found", "rmdir", "/missing")
	refused(t, bin, tg, "the step is begin or object", "rm", "/docs/x.txt", "--fail-after", "complete")
	refused(t, bin, tg, "rm -r takes no step", "rm", "-r", "/docs", "--fail-after", "begin")
	if out := ok(t, bin, tg, "rmdir", "/docs/sub"); !strings.HasPrefix(out, "rmdir: /docs/sub (id ") {
		t.Errorf("rmdir stdout = %q", out)
	}
	ok(t, bin, tg, "mkdir", "/owned", "--unit", unit)
	ok(t, bin, tg, "rmdir", "/owned")
	if got := column(ok(t, bin, tg, "ls", "/", "--unit", unit)); len(got) != 0 {
		t.Errorf("ls / as the unit after the rmdir = %v, want nothing", got)
	}

	// rm -r: a tree with files at two levels, the per-entry lines, the
	// summary, and the root refusal.
	ok(t, bin, tg, "mkdir", "/docs/x")
	ok(t, bin, tg, "mkdir", "/docs/x/y")
	ok(t, bin, tg, "put", local, "/docs/x/y/deep.txt")
	ok(t, bin, tg, "put", local, "/docs/top.txt")
	refused(t, bin, tg, "the root directory", "rm", "-r", "/")
	out := ok(t, bin, tg, "rm", "-r", "/docs")
	got := lines(out)
	want := []string{"rm: /docs/x/y/deep.txt", "rmdir: /docs/x/y", "rmdir: /docs/x", "rm: /docs/top.txt", "rmdir: /docs", "rm -r: /docs (2 files, 3 directories)"}
	if len(got) != len(want) {
		t.Fatalf("rm -r printed %d lines:\n%s", len(got), out)
	}
	for i, line := range got {
		if !strings.HasPrefix(line, want[i]) {
			t.Errorf("rm -r line %d = %q, want it to start with %q", i+1, line, want[i])
		}
	}
	if got := column(ok(t, bin, tg, "ls", "/")); len(got) != 0 {
		t.Errorf("ls / after rm -r = %v, want nothing", got)
	}
	refused(t, bin, tg, "not found", "rm", "-r", "/docs")
}
