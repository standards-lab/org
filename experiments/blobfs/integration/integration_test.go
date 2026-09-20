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

// run executes the binary with args and BLOBFS_DSN set to dsn for the child
// process, and returns its stdout, stderr, and exit code.
func run(t *testing.T, bin, dsn string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BLOBFS_DSN="+dsn)
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
func ok(t *testing.T, bin, dsn string, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, bin, dsn, args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

// refused runs the binary and fails the test unless it exits one with
// nothing on stdout and want on stderr.
func refused(t *testing.T, bin, dsn string, want string, args ...string) {
	t.Helper()
	out, errOut, code := run(t, bin, dsn, args...)
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
	db, dsn := livetest.OpenDSN(t)
	bin := build(t)

	out := ok(t, bin, dsn, "schema", "up")
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

	out = ok(t, bin, dsn, "schema", "down")
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
	db, dsn := livetest.OpenDSN(t)
	bin := build(t)
	unit, other := blobfs.NewID(), blobfs.NewID()

	refused(t, bin, dsn, "blobfs schema up", "ls", "/")
	ok(t, bin, dsn, "schema", "up")

	// mkdir.
	if out := ok(t, bin, dsn, "mkdir", "/reports"); !strings.HasPrefix(out, "mkdir: /reports (id ") {
		t.Errorf("mkdir stdout = %q", out)
	}
	ok(t, bin, dsn, "mkdir", "/reports/2026")
	ok(t, bin, dsn, "mkdir", "/reports/2025")
	if out := ok(t, bin, dsn, "mkdir", "/archive", "--unit", unit); !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("mkdir --unit stdout = %q, want the unit named", out)
	}
	ok(t, bin, dsn, "mkdir", "/archive/old")
	ok(t, bin, dsn, "mkdir", "/theirs", "--unit", other)
	refused(t, bin, dsn, "top-level directory only", "mkdir", "/reports/2024", "--unit", unit)
	refused(t, bin, dsn, "name taken", "mkdir", "/reports")
	refused(t, bin, dsn, "not found", "mkdir", "/missing/child")
	refused(t, bin, dsn, "the root directory", "mkdir", "/")
	refused(t, bin, dsn, "is not a UUID", "mkdir", "/x", "--unit", "nope")

	reports := directoryID(ctx, t, db, "reports")
	archive := directoryID(ctx, t, db, "archive")
	for i, name := range []string{"c.txt", "a.txt", "b.txt"} {
		insertFile(ctx, t, db, reports, name, int64(10*(3-i)))
	}
	insertFile(ctx, t, db, archive, "old.txt", 1)

	// ls: directories then files, one page each, with the totals.
	out := ok(t, bin, dsn, "ls", "/reports")
	if got := column(out); strings.Join(got, " ") != "2025 2026 a.txt b.txt c.txt" {
		t.Errorf("ls /reports names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total 2\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total 3\n") {
		t.Errorf("ls /reports stdout:\n%s", out)
	}
	out = ok(t, bin, dsn, "ls", "/reports", "--page", "2", "--size", "1", "--sort", "name:desc")
	if got := column(out); strings.Join(got, " ") != "2025 b.txt" {
		t.Errorf("ls page 2 of 1 by name desc names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 2 of size 1, total 2\n") || !strings.Contains(out, "files: 1 on page 2 of size 1, total 3\n") {
		t.Errorf("ls page 2 stdout:\n%s", out)
	}
	out = ok(t, bin, dsn, "ls", "/reports", "--sort", "size:desc")
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt a.txt b.txt" {
		t.Errorf("ls by size desc names = %v; want directories in name order and files by size", got)
	}
	out = ok(t, bin, dsn, "ls", "/reports", "--total", "none")
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total not counted\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total not counted\n") {
		t.Errorf("ls --total none stdout:\n%s", out)
	}
	out = ok(t, bin, dsn, "ls", "/reports", "--page", "5")
	if !strings.Contains(out, "directories: 0 on page 5 of size 20, total unknown") || !strings.Contains(out, "files: 0 on page 5 of size 20, total unknown") {
		t.Errorf("an empty later page stdout:\n%s", out)
	}
	out = ok(t, bin, dsn, "ls", "/reports/2026")
	if !strings.Contains(out, "directories: 0 on page 1 of size 20, total 0\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("an empty first page stdout:\n%s", out)
	}
	refused(t, bin, dsn, "not found", "ls", "/reports/missing")
	refused(t, bin, dsn, "unknown sort field", "ls", "/reports", "--sort", "owner")

	// The cursor: a half with a next page prints it, and the flag continues
	// that half alone, without a total.
	out = ok(t, bin, dsn, "ls", "/reports", "--size", "2")
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
	out = ok(t, bin, dsn, "ls", "/reports", "--size", "2", "--after-files", cursor)
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt" {
		t.Errorf("ls --after-files names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 2, total 2\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\n") || strings.Contains(out, "next-") {
		t.Errorf("ls --after-files stdout:\n%s", out)
	}
	refused(t, bin, dsn, "not one this listing issued", "ls", "/reports", "--after-files", "nonsense")
	refused(t, bin, dsn, "issued by files_in_directory", "ls", "/reports", "--after-dirs", cursor)
	refused(t, bin, dsn, "size can be NULL", "ls", "/reports", "--sort", "size", "--after-files", cursor)

	// --unit at a top-level directory and below it.
	out = ok(t, bin, dsn, "ls", "/archive", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "old old.txt" {
		t.Errorf("ls /archive as the owner names = %v", got)
	}
	ok(t, bin, dsn, "ls", "/archive/old", "--unit", unit)
	refused(t, bin, dsn, "does not own the directory", "ls", "/archive", "--unit", other)
	refused(t, bin, dsn, "does not own the directory", "ls", "/archive/old", "--unit", other)
	refused(t, bin, dsn, "does not own the directory", "ls", "/reports", "--unit", unit)

	// --unit at the root: the unit's own top-level directories.
	out = ok(t, bin, dsn, "ls", "/", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "archive" {
		t.Errorf("ls / as the unit names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total 1\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("ls / as the unit stdout:\n%s", out)
	}
	out = ok(t, bin, dsn, "ls", "/", "--unit", other)
	if got := column(out); strings.Join(got, " ") != "theirs" {
		t.Errorf("ls / as the other unit names = %v", got)
	}
	refused(t, bin, dsn, "owner read model takes no cursor", "ls", "/", "--unit", unit, "--after-dirs", cursor)
	out = ok(t, bin, dsn, "ls", "/")
	if got := column(out); strings.Join(got, " ") != "archive reports theirs" {
		t.Errorf("ls / names = %v", got)
	}
}
