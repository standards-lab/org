//go:build integration

// Package integration drives the built binary black-box against the compose
// stack: it builds cmd/blobfs once, runs it as a child process configured
// only through its environment and arguments, and checks the effect on a
// throwaway database and a throwaway container. TestScript is one ordered
// end-to-end script over every command family, run once per variant;
// TestIsolation runs two configurations side by side and shows neither
// sees the other. Every run logs its command line and its output, so go
// test -v prints the transcript. Every file carries the integration tag,
// so the package comment sits in this one.
package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// binary is the path of cmd/blobfs, built once by TestMain.
var binary string

// TestMain builds the binary into a temporary directory and removes the
// directory after the tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "blobfs-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "blobfs")
	cmd := exec.Command("go", "build", "-o", binary, "github.com/standards-lab/org/experiments/blobfs/cmd/blobfs")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// objectTables are the tables both migration sets create, in creation
// order.
var objectTables = []string{"blobfs_directory", "blobfs_file", "directory_owner", "bookmark"}

// historyTables are the two sets' history tables.
var historyTables = []string{"blobfs_schema_version", "schema_version"}

// target is what one configuration's runs of the binary point at: the
// throwaway database, through BLOBFS_DSN; the configuration's own
// container, through BLOBFS_STORAGE_CONTAINER; and the variant, through
// BLOBFS_VARIANT when set. The other storage settings come from the
// process's environment, which mise sets. label, when not empty,
// prefixes the transcript's lines, so two configurations tell apart.
// stdin, when not nil, is what the next run reads.
type target struct {
	dsn, container, variant, label string
	stdin                          io.Reader
}

// open creates a configuration's database and container and returns the
// session over the database and the target.
func open(t *testing.T, variant, label string) (*sqlate.DB, target) {
	t.Helper()
	db, dsn := livetest.OpenDSN(t)
	tg := target{dsn: dsn, container: livetest.Container(t), variant: variant, label: label}
	t.Logf("%sconfiguration: database %s, container %s, variant %q", tg.prefix(), livetest.DatabaseName(t, dsn), tg.container, variant)
	return db, tg
}

// prefix is the transcript prefix of a labelled target.
func (tg target) prefix() string {
	if tg.label == "" {
		return ""
	}
	return "[" + tg.label + "] "
}

// command builds the process for one run of the binary against tg.
func (tg target) command(args ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "BLOBFS_DSN="+tg.dsn, "BLOBFS_STORAGE_CONTAINER="+tg.container)
	if tg.variant != "" {
		cmd.Env = append(cmd.Env, "BLOBFS_VARIANT="+tg.variant)
	}
	cmd.Stdin = tg.stdin
	return cmd
}

// result is what one run of the binary produced.
type result struct {
	stdout, stderr string
	code           int
}

// finish turns a finished command's error into the exit code and fails on
// an error that is not the child's exit status.
func finish(t *testing.T, args []string, err error) int {
	t.Helper()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return -1
}

// transcript logs one run as a shell line and its output: stdout as it
// is, and the exit code with stderr when the run failed.
func transcript(t *testing.T, tg target, args []string, r result) {
	t.Helper()
	var b strings.Builder
	b.WriteString(tg.prefix() + "$ blobfs " + shellWords(args))
	if tg.stdin != nil {
		b.WriteString("  (with stdin)")
	}
	if r.stdout != "" {
		b.WriteString("\n" + strings.TrimRight(r.stdout, "\n"))
	}
	if r.code != 0 {
		b.WriteString(fmt.Sprintf("\nexit %d: %s", r.code, strings.TrimRight(r.stderr, "\n")))
	} else if r.stderr != "" {
		b.WriteString("\nstderr: " + strings.TrimRight(r.stderr, "\n"))
	}
	t.Log(b.String())
}

// shellWords joins args as a shell would read them, quoting a word that
// has a space or is empty.
func shellWords(args []string) string {
	words := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'") {
			a = fmt.Sprintf("%q", a)
		}
		words[i] = a
	}
	return strings.Join(words, " ")
}

// run executes the binary with args against tg, logs the transcript, and
// returns its stdout, stderr, and exit code.
func run(t *testing.T, tg target, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := tg.command(args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	code = finish(t, args, cmd.Run())
	r := result{out.String(), errOut.String(), code}
	transcript(t, tg, args, r)
	return r.stdout, r.stderr, r.code
}

// ok runs the binary and fails the test unless it exits zero, returning
// its stdout.
func ok(t *testing.T, tg target, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, tg, args...)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, errOut)
	}
	return out
}

// refused runs the binary and fails the test unless it exits one with
// nothing on stdout and want on stderr.
func refused(t *testing.T, tg target, want string, args ...string) {
	t.Helper()
	out, errOut, code := run(t, tg, args...)
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

// refusedByConstraint is refused for a refusal the database's constraint
// reported: stderr names the sentinel and the constraint, as want spells
// them, and carries none of the driver's text.
func refusedByConstraint(t *testing.T, tg target, want string, args ...string) {
	t.Helper()
	refused(t, tg, want, args...)
	_, errOut, _ := run(t, tg, args...)
	for _, text := range []string{"SQLSTATE", "duplicate key", "violates"} {
		if strings.Contains(errOut, text) {
			t.Errorf("%v: stderr = %q, want no driver text (%q)", args, errOut, text)
		}
	}
}

// putContent runs put from stdin with content as the file at path and
// returns its stdout.
func putContent(t *testing.T, tg target, content, path string, flags ...string) string {
	t.Helper()
	tg.stdin = strings.NewReader(content)
	return ok(t, tg, append([]string{"put", "-", path}, flags...)...)
}

// process is a run of the binary that the test started without waiting
// for it, so the test can observe whether it finishes while the test holds
// something.
type process struct {
	args []string
	tg   target
	done chan result
	err  chan error
}

// start begins a run of the binary against tg and returns the process.
func start(t *testing.T, tg target, args ...string) *process {
	t.Helper()
	cmd := tg.command(args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", args, err)
	}
	p := &process{args: args, tg: tg, done: make(chan result, 1), err: make(chan error, 1)}
	go func() {
		err := cmd.Wait()
		var exit *exec.ExitError
		code := 0
		switch {
		case err == nil:
		case errors.As(err, &exit):
			code = exit.ExitCode()
		default:
			p.err <- err
			return
		}
		p.done <- result{out.String(), errOut.String(), code}
	}()
	return p
}

// finished reports whether the process ended within d, logging its
// transcript when it did.
func (p *process) finished(t *testing.T, d time.Duration) (result, bool) {
	t.Helper()
	select {
	case r := <-p.done:
		transcript(t, p.tg, p.args, r)
		p.done <- r
		return r, true
	case err := <-p.err:
		t.Fatalf("run %v: %v", p.args, err)
	case <-time.After(d):
	}
	return result{}, false
}

// wait fails the test unless the process ends within d and exits zero,
// and returns its stdout.
func (p *process) wait(t *testing.T, d time.Duration) string {
	t.Helper()
	r, ended := p.finished(t, d)
	if !ended {
		t.Fatalf("%v did not finish within %s", p.args, d)
	}
	if r.code != 0 {
		t.Fatalf("%v exited %d: %s", p.args, r.code, r.stderr)
	}
	return r.stdout
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

// field returns the value of one label of a stat record.
func field(out, label string) string {
	for _, line := range lines(out) {
		if rest, found := strings.CutPrefix(line, label+":"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
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

// strings1 runs a one-column query on db and returns the column's values.
func strings1(ctx context.Context, t *testing.T, db *sqlate.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return values
}

// count runs a count query on db and returns the count.
func count(ctx context.Context, t *testing.T, db *sqlate.DB, query string, args ...any) int {
	t.Helper()
	got := strings1(ctx, t, db, query, args...)
	if len(got) != 1 {
		t.Fatalf("%s: %d rows, want one", query, len(got))
	}
	var n int
	if _, err := fmt.Sscan(got[0], &n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// localFile writes content to a file in the test's temporary directory
// and returns its path.
func localFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// variants are the two variants the script runs over, with whether the
// variant's tree lock serializes directory moves, which is the one
// difference the binary shows.
var variants = []struct {
	name       string
	serializes bool
}{
	{"standard", false},
	{"postgres", true},
}

// script is one end-to-end run of the binary over one configuration: its
// database, its target, and what the steps share.
type script struct {
	ctx        context.Context
	db         *sqlate.DB
	tg         target
	serializes bool
	// local is a local file holding "quarterly\n", what put uploads.
	local string
}

// TestScript is the scripted run of every command family through the
// built binary, once per variant, each in its own database and container:
// the steps below in order, each a subtest, and the script stops at the
// first step that fails. The transcript under -v is the record of what
// the binary does; mise run demo prints it.
func TestScript(t *testing.T) {
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			db, tg := open(t, v.name, "")
			s := &script{ctx: context.Background(), db: db, tg: tg, serializes: v.serializes, local: localFile(t, "report.txt", "quarterly\n")}
			for _, step := range []struct {
				name string
				fn   func(*testing.T)
			}{
				{"schema-up", s.schemaUp},
				{"directories", s.directories},
				{"writes", s.writes},
				{"bookmarks", s.bookmarks},
				{"deletes", s.deletes},
				{"moves", s.moves},
				{"tree-lock", s.treeLock},
				{"schema-down", s.schemaDown},
			} {
				if !t.Run(step.name, step.fn) {
					t.Logf("the script stops at step %s", step.name)
					return
				}
			}
		})
	}
}

// schemaUp is the first step: a listing before the schema fails and names
// schema up; status on the empty database lists every migration pending;
// up creates the consumer's tables over blobfs's and seeds the root and
// prints its one result line; status then shows both sets at head.
func (s *script) schemaUp(t *testing.T) {
	refused(t, s.tg, "blobfs schema up", "ls", "/")
	out := ok(t, s.tg, "schema", "status")
	if got := lines(out); len(got) != 3 || !strings.HasPrefix(got[0], "set ") || !strings.Contains(got[1], "2 file") || !strings.Contains(got[2], "2 bookmark") {
		t.Errorf("schema status on an empty database:\n%s", out)
	}
	out = ok(t, s.tg, "schema", "up")
	if !strings.HasPrefix(out, "schema up:") {
		t.Errorf("schema up stdout = %q, want the result line", out)
	}
	for _, table := range objectTables {
		if !livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("after schema up, table %s is missing", table)
		}
	}
	if roots := count(s.ctx, t, s.db, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id IS NULL"); roots != 1 {
		t.Errorf("after schema up, %d roots, want the one seeded root", roots)
	}
	s.statusAtHead(t)
}

// statusAtHead runs schema status and checks both sets are at head with
// nothing pending and nothing dirty.
func (s *script) statusAtHead(t *testing.T) {
	t.Helper()
	out := ok(t, s.tg, "schema", "status")
	got := lines(out)
	if len(got) != 3 {
		t.Fatalf("schema status:\n%s", out)
	}
	if f := strings.Fields(got[1]); len(f) != 6 || f[0] != "blobfs" || f[1] != "blobfs_schema_version" || f[2] != "2" || f[3] != "2" || f[4] != "none" || f[5] != "false" {
		t.Errorf("blobfs status row = %q", got[1])
	}
	if f := strings.Fields(got[2]); len(f) != 6 || f[0] != "consumer" || f[1] != "schema_version" || f[2] != "2" || f[3] != "2" || f[4] != "none" || f[5] != "false" {
		t.Errorf("consumer status row = %q", got[2])
	}
}

// directories is the directory commands: the variant flag, mkdir at
// nested paths and with --unit at a top-level path, the refusals mkdir
// renders, then ls with paging, sorting, --total none, the empty later
// page, the cursor, and --unit scoping at a top-level directory and at
// the root. The files are put from stdin with sizes that the size sort
// shows.
func (s *script) directories(t *testing.T) {
	unit, other := blobfs.NewID(), blobfs.NewID()

	// The variant: the flag beats the environment, and a name that is
	// neither is refused before any I/O.
	refused(t, s.tg, "--variant or BLOBFS_VARIANT is standard or postgres", "ls", "/", "--variant", "nope")
	ok(t, s.tg, "ls", "/", "--variant", s.tg.variant)

	// mkdir.
	if out := ok(t, s.tg, "mkdir", "/reports"); !strings.HasPrefix(out, "mkdir: /reports (id ") {
		t.Errorf("mkdir stdout = %q", out)
	}
	ok(t, s.tg, "mkdir", "/reports/2026")
	ok(t, s.tg, "mkdir", "/reports/2025")
	if out := ok(t, s.tg, "mkdir", "/archive", "--unit", unit); !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("mkdir --unit stdout = %q, want the unit named", out)
	}
	ok(t, s.tg, "mkdir", "/archive/old")
	ok(t, s.tg, "mkdir", "/theirs", "--unit", other)
	refused(t, s.tg, "top-level directory only", "mkdir", "/reports/2024", "--unit", unit)
	refusedByConstraint(t, s.tg, "blobfs: name taken (constraint blobfs_uq_directory_parent_name)", "mkdir", "/reports")
	refused(t, s.tg, "not found", "mkdir", "/missing/child")
	refused(t, s.tg, "the root directory", "mkdir", "/")
	refused(t, s.tg, "is not a UUID", "mkdir", "/x", "--unit", "nope")

	putContent(t, s.tg, strings.Repeat("c", 30), "/reports/c.txt")
	putContent(t, s.tg, strings.Repeat("a", 20), "/reports/a.txt")
	putContent(t, s.tg, strings.Repeat("b", 10), "/reports/b.txt")
	putContent(t, s.tg, "o", "/archive/old.txt")

	// ls: directories then files, one page each, with the totals.
	out := ok(t, s.tg, "ls", "/reports")
	if got := column(out); strings.Join(got, " ") != "2025 2026 a.txt b.txt c.txt" {
		t.Errorf("ls /reports names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total 2\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total 3\n") {
		t.Errorf("ls /reports stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports", "--page", "2", "--size", "1", "--sort", "name:desc")
	if got := column(out); strings.Join(got, " ") != "2025 b.txt" {
		t.Errorf("ls page 2 of 1 by name desc names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 2 of size 1, total 2\n") || !strings.Contains(out, "files: 1 on page 2 of size 1, total 3\n") {
		t.Errorf("ls page 2 stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports", "--sort", "size:desc")
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt a.txt b.txt" {
		t.Errorf("ls by size desc names = %v; want directories in name order and files by size", got)
	}
	out = ok(t, s.tg, "ls", "/reports", "--total", "none")
	if !strings.Contains(out, "directories: 2 on page 1 of size 20, total not counted\n") || !strings.Contains(out, "files: 3 on page 1 of size 20, total not counted\n") {
		t.Errorf("ls --total none stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports", "--page", "5")
	if !strings.Contains(out, "directories: 0 on page 5 of size 20, total unknown") || !strings.Contains(out, "files: 0 on page 5 of size 20, total unknown") {
		t.Errorf("an empty later page stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/reports/2026")
	if !strings.Contains(out, "directories: 0 on page 1 of size 20, total 0\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("an empty first page stdout:\n%s", out)
	}
	refused(t, s.tg, "not found", "ls", "/reports/missing")
	refused(t, s.tg, "unknown sort field", "ls", "/reports", "--sort", "owner")

	// The cursor: a half with a next page prints it, and the flag continues
	// that half alone, without a total.
	out = ok(t, s.tg, "ls", "/reports", "--size", "2")
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
	out = ok(t, s.tg, "ls", "/reports", "--size", "2", "--after-files", cursor)
	if got := column(out); strings.Join(got, " ") != "2025 2026 c.txt" {
		t.Errorf("ls --after-files names = %v", got)
	}
	if !strings.Contains(out, "directories: 2 on page 1 of size 2, total 2\n") || !strings.Contains(out, "files: 1 after the cursor, size 2, total not counted\n") || strings.Contains(out, "next-") {
		t.Errorf("ls --after-files stdout:\n%s", out)
	}
	refused(t, s.tg, "not one this listing issued", "ls", "/reports", "--after-files", "nonsense")
	refused(t, s.tg, "issued by files_in_directory", "ls", "/reports", "--after-dirs", cursor)
	refused(t, s.tg, "size can be NULL", "ls", "/reports", "--sort", "size", "--after-files", cursor)

	// --unit at a top-level directory and below it.
	out = ok(t, s.tg, "ls", "/archive", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "old old.txt" {
		t.Errorf("ls /archive as the owner names = %v", got)
	}
	ok(t, s.tg, "ls", "/archive/old", "--unit", unit)
	refused(t, s.tg, "does not own the directory", "ls", "/archive", "--unit", other)
	refused(t, s.tg, "does not own the directory", "ls", "/archive/old", "--unit", other)
	refused(t, s.tg, "does not own the directory", "ls", "/reports", "--unit", unit)

	// --unit at the root: the unit's own top-level directories.
	out = ok(t, s.tg, "ls", "/", "--unit", unit)
	if got := column(out); strings.Join(got, " ") != "archive" {
		t.Errorf("ls / as the unit names = %v", got)
	}
	if !strings.Contains(out, "directories: 1 on page 1 of size 20, total 1\n") || !strings.Contains(out, "files: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("ls / as the unit stdout:\n%s", out)
	}
	out = ok(t, s.tg, "ls", "/", "--unit", other)
	if got := column(out); strings.Join(got, " ") != "theirs" {
		t.Errorf("ls / as the other unit names = %v", got)
	}
	refused(t, s.tg, "owner read model takes no cursor", "ls", "/", "--unit", unit, "--after-dirs", cursor)
	out = ok(t, s.tg, "ls", "/")
	if got := column(out); strings.Join(got, " ") != "archive reports theirs" {
		t.Errorf("ls / names = %v", got)
	}
}

// writes is the file commands against Postgres and Azurite: put from a
// local file and from stdin, cat returning the bytes put stored, stat
// showing the row, the refusals put renders, and --fail-after at each
// step: the binary exits non-zero with the row left pending, which stat
// and ls show and cat refuses, and a put of the same path resumes and
// completes it. A name at blobfs's rune limit is accepted and one over is
// refused.
func (s *script) writes(t *testing.T) {
	ok(t, s.tg, "mkdir", "/docs")

	// put, cat, stat.
	out := ok(t, s.tg, "put", s.local, "/docs/report.txt")
	if !strings.HasPrefix(out, "put: /docs/report.txt (id ") || !strings.Contains(out, ", 10 bytes, etag \"") {
		t.Errorf("put stdout = %q", out)
	}
	if out := ok(t, s.tg, "cat", "/docs/report.txt"); out != "quarterly\n" {
		t.Errorf("cat stdout = %q", out)
	}
	out = ok(t, s.tg, "stat", "/docs/report.txt")
	if field(out, "status") != "available" || field(out, "size") != "10" || !strings.HasPrefix(field(out, "content-type"), "text/plain") ||
		!strings.HasPrefix(field(out, "etag"), "\"") || field(out, "version") != "2" || field(out, "name") != "report.txt" {
		t.Errorf("stat stdout:\n%s", out)
	}
	if out := putContent(t, s.tg, "from stdin", "/docs/notes.md", "--content-type", "text/markdown"); !strings.Contains(out, ", 10 bytes, etag ") {
		t.Errorf("put - stdout = %q", out)
	}
	if out := ok(t, s.tg, "cat", "/docs/notes.md"); out != "from stdin" {
		t.Errorf("cat of the stdin put = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/docs/notes.md"); field(out, "content-type") != "text/markdown" {
		t.Errorf("stat of the stdin put:\n%s", out)
	}
	if got := column(ok(t, s.tg, "ls", "/docs")); strings.Join(got, " ") != "notes.md report.txt" {
		t.Errorf("ls /docs names = %v", got)
	}

	// Refusals.
	refused(t, s.tg, "name taken", "put", s.local, "/docs/report.txt")
	refused(t, s.tg, "not found", "put", s.local, "/missing/report.txt")
	refused(t, s.tg, "not found", "cat", "/docs/missing.txt")
	refused(t, s.tg, "not found", "stat", "/docs/missing.txt")
	refused(t, s.tg, "the step is insert or write", "put", s.local, "/docs/x.txt", "--fail-after", "complete")
	refused(t, s.tg, "the root directory", "put", s.local, "/")

	// --fail-after insert: the row is pending and visible, nothing is
	// stored, and the retry completes it.
	_, errOut, code := run(t, s.tg, "put", s.local, "/docs/stopped.txt", "--fail-after", "insert")
	if code != 1 || !strings.Contains(errOut, "stopped after step insert") || !strings.Contains(errOut, "rerun put") {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}
	out = ok(t, s.tg, "stat", "/docs/stopped.txt")
	if field(out, "status") != "pending" || field(out, "size") != "-" || field(out, "etag") != "-" || field(out, "version") != "1" {
		t.Errorf("stat after the stop:\n%s", out)
	}
	listed := false
	for _, line := range lines(ok(t, s.tg, "ls", "/docs")) {
		if f := strings.Fields(line); len(f) > 3 && f[1] == "stopped.txt" {
			listed = f[2] == "-" && f[3] == "pending"
		}
	}
	if !listed {
		t.Errorf("ls after the stop does not show stopped.txt pending with no size")
	}
	refused(t, s.tg, "the file is pending", "cat", "/docs/stopped.txt")
	if got := strings1(s.ctx, t, s.db, "SELECT status FROM blobfs_file WHERE name = 'stopped.txt'"); strings.Join(got, ",") != "pending" {
		t.Errorf("the database holds status %v for the stopped put", got)
	}
	out = ok(t, s.tg, "put", s.local, "/docs/stopped.txt")
	if !strings.HasSuffix(out, ", resumed the pending row)\n") {
		t.Errorf("the retry stdout = %q, want the resumed row", out)
	}
	if out := ok(t, s.tg, "cat", "/docs/stopped.txt"); out != "quarterly\n" {
		t.Errorf("cat after the retry = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/docs/stopped.txt"); field(out, "status") != "available" || field(out, "version") != "2" {
		t.Errorf("stat after the retry:\n%s", out)
	}

	// --fail-after write: the object is stored, the row is still pending,
	// and the retry completes it.
	_, errOut, code = run(t, s.tg, "put", s.local, "/docs/written.txt", "--fail-after", "write")
	if code != 1 || !strings.Contains(errOut, "stopped after step write") {
		t.Fatalf("put --fail-after write exited %d: %s", code, errOut)
	}
	if out := ok(t, s.tg, "stat", "/docs/written.txt"); field(out, "status") != "pending" {
		t.Errorf("stat after the stop after write:\n%s", out)
	}
	if out := ok(t, s.tg, "put", s.local, "/docs/written.txt"); !strings.Contains(out, "resumed the pending row") {
		t.Errorf("the retry after write = %q", out)
	}
	if out := ok(t, s.tg, "cat", "/docs/written.txt"); out != "quarterly\n" {
		t.Errorf("cat after the retry = %q", out)
	}

	// The rune boundary of a name.
	name := strings.Repeat("\u00e9", blobfs.MaxNameLength)
	ok(t, s.tg, "put", s.local, "/docs/"+name)
	if out := ok(t, s.tg, "cat", "/docs/"+name); out != "quarterly\n" {
		t.Errorf("cat of the boundary name = %q", out)
	}
	refused(t, s.tg, "at most 255 characters", "put", s.local, "/docs/"+name+"\u00e9")
}

// bookmarks is the bookmark commands: add at depth six and at the root,
// the active bookmark and the refusal of a second one, the duplicate add,
// the missing file, ls with its full paths, the active marker, the
// pending file, paging, sorting, --total none, and the unit filter, then
// rm of the active bookmark and the refusals rm renders.
func (s *script) bookmarks(t *testing.T) {
	unit, other := blobfs.NewID(), blobfs.NewID()
	dir := ""
	for _, name := range []string{"d1", "d2", "d3", "d4", "d5", "d6"} {
		dir += "/" + name
		ok(t, s.tg, "mkdir", dir)
	}
	ok(t, s.tg, "mkdir", "/library")
	deep := dir + "/plan.txt"
	ok(t, s.tg, "put", s.local, deep)
	ok(t, s.tg, "put", s.local, "/root.txt")
	ok(t, s.tg, "put", s.local, "/library/x.txt")
	ok(t, s.tg, "put", s.local, "/library/y.txt")
	if _, errOut, code := run(t, s.tg, "put", s.local, "/library/draft.bin", "--fail-after", "insert"); code != 1 {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}

	// add: the result line, the active bookmark, and the refusals.
	out := ok(t, s.tg, "bookmark", "add", deep, "--unit", unit)
	if !strings.HasPrefix(out, "bookmark add: "+deep+" (file ") || !strings.HasSuffix(out, ", unit "+unit+", inactive)\n") {
		t.Errorf("bookmark add stdout = %q", out)
	}
	if out := ok(t, s.tg, "bookmark", "add", "/library/x.txt", "--unit", unit, "--active"); !strings.HasSuffix(out, ", unit "+unit+", active)\n") {
		t.Errorf("bookmark add --active stdout = %q", out)
	}
	ok(t, s.tg, "bookmark", "add", "/root.txt", "--unit", unit)
	ok(t, s.tg, "bookmark", "add", "/library/draft.bin", "--unit", unit)
	ok(t, s.tg, "bookmark", "add", "/library/x.txt", "--unit", other, "--active")
	refused(t, s.tg, "has an active bookmark already", "bookmark", "add", "/library/y.txt", "--unit", unit, "--active")
	refused(t, s.tg, "has bookmarked the file already", "bookmark", "add", "/root.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "add", "/library/missing.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "add", "/library", "--unit", unit)
	refused(t, s.tg, `required flag(s) "unit" not set`, "bookmark", "add", "/root.txt")
	refused(t, s.tg, "is not a UUID", "bookmark", "add", "/root.txt", "--unit", "nope")

	// ls: full paths in path order, the active marker, the pending file
	// with its status, the total; then paging, sorting, --total none, and
	// the other unit's own listing.
	out = ok(t, s.tg, "bookmark", "ls", "--unit", unit)
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep+" /library/draft.bin /library/x.txt /root.txt" {
		t.Errorf("bookmark ls paths = %v", got)
	}
	if !strings.Contains(out, "bookmarks: 4 on page 1 of size 20, total 4\n") {
		t.Errorf("bookmark ls stdout:\n%s", out)
	}
	for _, line := range lines(out) {
		f := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "/library/x.txt") && (f[2] != "available" || f[3] != "active"):
			t.Errorf("the active bookmark's line = %q", line)
		case strings.HasPrefix(line, "/library/draft.bin") && (f[1] != "-" || f[2] != "pending" || f[3] != "-"):
			t.Errorf("the pending file's line = %q", line)
		case strings.HasPrefix(line, deep) && (f[1] != "10" || f[3] != "-"):
			t.Errorf("the deep file's line = %q", line)
		}
	}
	out = ok(t, s.tg, "bookmark", "ls", "--unit", unit, "--page", "2", "--size", "3", "--sort", "path:desc")
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep {
		t.Errorf("bookmark ls page 2 of 3 by path desc = %v", got)
	}
	if !strings.Contains(out, "bookmarks: 1 on page 2 of size 3, total 4\n") {
		t.Errorf("bookmark ls page 2 stdout:\n%s", out)
	}
	out = ok(t, s.tg, "bookmark", "ls", "--unit", unit, "--total", "none")
	if !strings.Contains(out, "bookmarks: 4 on page 1 of size 20, total not counted\n") {
		t.Errorf("bookmark ls --total none stdout:\n%s", out)
	}
	out = ok(t, s.tg, "bookmark", "ls", "--unit", other)
	if got := bookmarkPaths(out); strings.Join(got, " ") != "/library/x.txt" {
		t.Errorf("bookmark ls of the other unit = %v", got)
	}
	out = ok(t, s.tg, "bookmark", "ls", "--unit", blobfs.NewID())
	if !strings.Contains(out, "bookmarks: 0 on page 1 of size 20, total 0\n") {
		t.Errorf("bookmark ls of a unit with none:\n%s", out)
	}
	refused(t, s.tg, "unknown sort field", "bookmark", "ls", "--unit", unit, "--sort", "key")
	refused(t, s.tg, `required flag(s) "unit" not set`, "bookmark", "ls")

	// rm: the active bookmark goes, another can become active, and the
	// refusals.
	out = ok(t, s.tg, "bookmark", "rm", "/library/x.txt", "--unit", unit)
	if !strings.HasPrefix(out, "bookmark rm: /library/x.txt (file ") || !strings.HasSuffix(out, ", unit "+unit+")\n") {
		t.Errorf("bookmark rm stdout = %q", out)
	}
	ok(t, s.tg, "bookmark", "add", "/library/y.txt", "--unit", unit, "--active")
	refused(t, s.tg, "has no bookmark of the file", "bookmark", "rm", "/library/x.txt", "--unit", unit)
	refused(t, s.tg, "not found", "bookmark", "rm", "/library/missing.txt", "--unit", unit)
	out = ok(t, s.tg, "bookmark", "ls", "--unit", unit)
	if got := bookmarkPaths(out); strings.Join(got, " ") != deep+" /library/draft.bin /library/y.txt /root.txt" {
		t.Errorf("bookmark ls after rm = %v", got)
	}
	if got := bookmarkPaths(ok(t, s.tg, "bookmark", "ls", "--unit", other)); strings.Join(got, " ") != "/library/x.txt" {
		t.Errorf("the other unit's bookmark after the unit's rm = %v", got)
	}
}

// deletes is the delete commands against Postgres and Azurite: rm
// --fail-after at each step, the deleting row that stat and ls show and
// cat and put refuse, the rm that finishes it, the bookmark that refuses
// an rm until it is removed, rm of an abandoned put, rmdir of an empty and
// of a non-empty directory and of an owned one, the root refusals, and rm
// -r over a tree with its per-entry lines and its summary.
func (s *script) deletes(t *testing.T) {
	unit := blobfs.NewID()
	ok(t, s.tg, "mkdir", "/trash")
	ok(t, s.tg, "mkdir", "/trash/sub")
	ok(t, s.tg, "put", s.local, "/trash/a.txt")
	ok(t, s.tg, "put", s.local, "/trash/b.txt")
	ok(t, s.tg, "put", s.local, "/trash/sub/c.txt")

	// --fail-after begin: the row is deleting and visible, and the retry
	// finishes the delete.
	_, errOut, code := run(t, s.tg, "rm", "/trash/a.txt", "--fail-after", "begin")
	if code != 1 || !strings.Contains(errOut, "stopped after step begin") || !strings.Contains(errOut, "the row is deleting") || !strings.Contains(errOut, "rerun rm") {
		t.Fatalf("rm --fail-after begin exited %d: %s", code, errOut)
	}
	if out := ok(t, s.tg, "stat", "/trash/a.txt"); field(out, "status") != "deleting" || field(out, "version") != "3" {
		t.Errorf("stat after the stop:\n%s", out)
	}
	listed := false
	for _, line := range lines(ok(t, s.tg, "ls", "/trash")) {
		if f := strings.Fields(line); len(f) > 3 && f[1] == "a.txt" {
			listed = f[3] == "deleting"
		}
	}
	if !listed {
		t.Errorf("ls after the stop does not show a.txt deleting")
	}
	refused(t, s.tg, "the file is deleting", "cat", "/trash/a.txt")
	refused(t, s.tg, "name taken", "put", s.local, "/trash/a.txt")
	if out := ok(t, s.tg, "rm", "/trash/a.txt"); !strings.HasPrefix(out, "rm: /trash/a.txt (id ") {
		t.Errorf("the finishing rm stdout = %q", out)
	}
	refused(t, s.tg, "not found", "stat", "/trash/a.txt")
	refused(t, s.tg, "not found", "rm", "/trash/a.txt")

	// --fail-after object: the object is gone, the row stays deleting, and
	// the retry removes it.
	_, errOut, code = run(t, s.tg, "rm", "/trash/b.txt", "--fail-after", "object")
	if code != 1 || !strings.Contains(errOut, "stopped after step object") {
		t.Fatalf("rm --fail-after object exited %d: %s", code, errOut)
	}
	if out := ok(t, s.tg, "stat", "/trash/b.txt"); field(out, "status") != "deleting" {
		t.Errorf("stat after the stop after object:\n%s", out)
	}
	ok(t, s.tg, "rm", "/trash/b.txt")
	refused(t, s.tg, "not found", "stat", "/trash/b.txt")
	if remaining := count(s.ctx, t, s.db, "SELECT COUNT(*) FROM blobfs_file WHERE name IN ('a.txt', 'b.txt') AND directory_id = (SELECT id FROM blobfs_directory WHERE name = 'trash')"); remaining != 0 {
		t.Errorf("%d rows of the removed files remain", remaining)
	}

	// A bookmark refuses the rm until it is removed.
	ok(t, s.tg, "bookmark", "add", "/trash/sub/c.txt", "--unit", unit)
	refused(t, s.tg, "the file is bookmarked", "rm", "/trash/sub/c.txt")
	if out := ok(t, s.tg, "stat", "/trash/sub/c.txt"); field(out, "status") != "available" {
		t.Errorf("stat after the refused rm:\n%s", out)
	}
	ok(t, s.tg, "bookmark", "rm", "/trash/sub/c.txt", "--unit", unit)
	ok(t, s.tg, "rm", "/trash/sub/c.txt")

	// An abandoned put is removed the same way.
	if _, errOut, code := run(t, s.tg, "put", s.local, "/trash/abandoned.txt", "--fail-after", "insert"); code != 1 {
		t.Fatalf("put --fail-after insert exited %d: %s", code, errOut)
	}
	ok(t, s.tg, "rm", "/trash/abandoned.txt")
	refused(t, s.tg, "not found", "stat", "/trash/abandoned.txt")

	// rmdir: the refusals, then an empty directory and an owned one.
	refusedByConstraint(t, s.tg, "blobfs: directory not empty (constraint blobfs_fk_", "rmdir", "/trash")
	refused(t, s.tg, "the root directory", "rmdir", "/")
	refused(t, s.tg, "not found", "rmdir", "/missing")
	refused(t, s.tg, "the step is begin or object", "rm", "/trash/x.txt", "--fail-after", "complete")
	refused(t, s.tg, "rm -r takes no step", "rm", "-r", "/trash", "--fail-after", "begin")
	if out := ok(t, s.tg, "rmdir", "/trash/sub"); !strings.HasPrefix(out, "rmdir: /trash/sub (id ") {
		t.Errorf("rmdir stdout = %q", out)
	}
	ok(t, s.tg, "mkdir", "/owned", "--unit", unit)
	ok(t, s.tg, "rmdir", "/owned")
	if got := column(ok(t, s.tg, "ls", "/", "--unit", unit)); len(got) != 0 {
		t.Errorf("ls / as the unit after the rmdir = %v, want nothing", got)
	}

	// rm -r: a tree with files at two levels, the per-entry lines, the
	// summary, and the root refusal.
	ok(t, s.tg, "mkdir", "/trash/x")
	ok(t, s.tg, "mkdir", "/trash/x/y")
	ok(t, s.tg, "put", s.local, "/trash/x/y/deep.txt")
	ok(t, s.tg, "put", s.local, "/trash/top.txt")
	refused(t, s.tg, "the root directory", "rm", "-r", "/")
	out := ok(t, s.tg, "rm", "-r", "/trash")
	got := lines(out)
	want := []string{"rm: /trash/x/y/deep.txt", "rmdir: /trash/x/y", "rmdir: /trash/x", "rm: /trash/top.txt", "rmdir: /trash", "rm -r: /trash (2 files, 3 directories)"}
	if len(got) != len(want) {
		t.Fatalf("rm -r printed %d lines:\n%s", len(got), out)
	}
	for i, line := range got {
		if !strings.HasPrefix(line, want[i]) {
			t.Errorf("rm -r line %d = %q, want it to start with %q", i+1, line, want[i])
		}
	}
	for _, name := range column(ok(t, s.tg, "ls", "/")) {
		if name == "trash" {
			t.Errorf("ls / after rm -r still lists trash")
		}
	}
	refused(t, s.tg, "not found", "rm", "-r", "/trash")
}

// moves is mv: a file into an existing directory and renamed, with cat
// reading the same bytes at the new path; a directory into an existing
// directory and renamed, with ls showing the contents at the new path;
// the cycle refusals; the root refusal; the scope rule, with a top-level
// directory renamed under its unit and refused below another; the taken
// name and the missing source; and a bookmark whose listed path follows
// the file.
func (s *script) moves(t *testing.T) {
	unit := blobfs.NewID()
	local := localFile(t, "f.txt", "moved\n")
	for _, p := range []string{"/a", "/a/x", "/a/y", "/b"} {
		ok(t, s.tg, "mkdir", p)
	}
	ok(t, s.tg, "put", local, "/a/x/f.txt")

	// A file into a directory, then renamed.
	if out := ok(t, s.tg, "mv", "/a/x/f.txt", "/a/y"); !strings.HasPrefix(out, "mv: /a/x/f.txt -> /a/y/f.txt (id ") {
		t.Errorf("mv stdout = %q", out)
	}
	if out := ok(t, s.tg, "cat", "/a/y/f.txt"); out != "moved\n" {
		t.Errorf("cat after the move = %q", out)
	}
	if out := ok(t, s.tg, "mv", "/a/y/f.txt", "/a/y/g.txt"); !strings.HasPrefix(out, "mv: /a/y/f.txt -> /a/y/g.txt (id ") {
		t.Errorf("mv rename stdout = %q", out)
	}
	if out := ok(t, s.tg, "cat", "/a/y/g.txt"); out != "moved\n" {
		t.Errorf("cat after the rename = %q", out)
	}
	if out := ok(t, s.tg, "stat", "/a/y/g.txt"); field(out, "name") != "g.txt" || field(out, "version") != "4" {
		t.Errorf("stat after two moves:\n%s", out)
	}
	refused(t, s.tg, "not found", "stat", "/a/x/f.txt")

	// A directory into a directory, then renamed.
	if out := ok(t, s.tg, "mv", "/a/x", "/a/y"); !strings.HasPrefix(out, "mv: /a/x -> /a/y/x (id ") {
		t.Errorf("mv of a directory stdout = %q", out)
	}
	if got := column(ok(t, s.tg, "ls", "/a/y")); strings.Join(got, " ") != "x g.txt" {
		t.Errorf("ls /a/y after the move = %v", got)
	}
	ok(t, s.tg, "mv", "/a/y/x", "/a/y/z")
	if got := column(ok(t, s.tg, "ls", "/a/y")); strings.Join(got, " ") != "z g.txt" {
		t.Errorf("ls /a/y after the rename = %v", got)
	}

	// Refusals: the cycle, the root, the scope rule, the taken name, and
	// the missing source.
	refused(t, s.tg, "would create a cycle", "mv", "/a/y", "/a/y/z")
	refused(t, s.tg, "would create a cycle", "mv", "/a/y", "/a/y")
	refused(t, s.tg, "the root directory", "mv", "/", "/elsewhere")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a/y", "/b/y")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a", "/b")
	refused(t, s.tg, "stays under one top-level directory", "mv", "/a/y/z", "/z")
	ok(t, s.tg, "mkdir", "/owned", "--unit", unit)
	ok(t, s.tg, "mv", "/owned", "/renamed")
	if got := column(ok(t, s.tg, "ls", "/", "--unit", unit)); strings.Join(got, " ") != "renamed" {
		t.Errorf("ls / as the unit after the rename = %v", got)
	}
	refused(t, s.tg, "stays under one top-level directory", "mv", "/renamed", "/b/renamed")
	ok(t, s.tg, "mkdir", "/a/held")
	ok(t, s.tg, "mkdir", "/a/y/held")
	refusedByConstraint(t, s.tg, "blobfs: name taken (constraint blobfs_uq_directory_parent_name)", "mv", "/a/held", "/a/y")
	refused(t, s.tg, "not found", "mv", "/a/missing", "/a/y")
	refused(t, s.tg, "not found", "mv", "/a/held", "/a/nope/held")

	// A bookmark's listed path follows the file.
	ok(t, s.tg, "bookmark", "add", "/a/y/g.txt", "--unit", unit)
	ok(t, s.tg, "mv", "/a/y/g.txt", "/a/y/z/h.txt")
	if got := bookmarkPaths(ok(t, s.tg, "bookmark", "ls", "--unit", unit)); strings.Join(got, " ") != "/a/y/z/h.txt" {
		t.Errorf("bookmark ls after the move = %v", got)
	}
	if out := ok(t, s.tg, "cat", "/a/y/z/h.txt"); out != "moved\n" {
		t.Errorf("cat after the third move = %q", out)
	}
}

// treeLock is the one step where the binary shows which variant it runs:
// the test holds blobfs's tree lock (the advisory lock under
// postgres.TreeLockKey) in a transaction of its own and runs two moves
// against it. A file move takes no lock on either variant and completes
// while the lock is held. A directory move on postgres takes the lock
// inside its transaction and waits until the test's transaction ends; on
// the standard baseline the lock is a no-op, and the move completes while
// the lock is held. Either way the move then succeeds and ls shows the
// tree.
func (s *script) treeLock(t *testing.T) {
	for _, p := range []string{"/locked", "/locked/x", "/locked/y"} {
		ok(t, s.tg, "mkdir", p)
	}
	ok(t, s.tg, "put", s.local, "/locked/x/f.txt")

	tx, err := s.db.Begin(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = tx.Rollback()
		}
	}
	defer release()
	if _, err := tx.ExecContext(s.ctx, "SELECT pg_advisory_xact_lock($1)", postgres.TreeLockKey); err != nil {
		t.Fatal(err)
	}
	t.Logf("the test holds the tree lock (pg_advisory_xact_lock(%d), postgres.TreeLockKey) in a transaction of its own", postgres.TreeLockKey)

	// A file move takes no lock on either variant.
	file := start(t, s.tg, "mv", "/locked/x/f.txt", "/locked/y")
	if out := file.wait(t, 20*time.Second); !strings.HasPrefix(out, "mv: /locked/x/f.txt -> /locked/y/f.txt") {
		t.Errorf("the file move stdout = %q", out)
	}
	t.Log("the file move completed while the lock was held: a file move takes no lock on either variant")

	// A directory move takes the variant's lock.
	dir := start(t, s.tg, "mv", "/locked/x", "/locked/y")
	if s.serializes {
		if r, ended := dir.finished(t, 2*time.Second); ended {
			t.Fatalf("on %s the directory move completed while the tree lock was held: %+v", s.tg.variant, r)
		}
		t.Logf("on %s the directory move is still waiting after 2 s: its transaction takes the tree lock the test holds", s.tg.variant)
		release()
		t.Log("the test's transaction ends and releases the lock")
	} else {
		t.Logf("on %s the directory move takes no lock, so it completes while the test holds the lock", s.tg.variant)
	}
	if out := dir.wait(t, 20*time.Second); !strings.HasPrefix(out, "mv: /locked/x -> /locked/y/x") {
		t.Errorf("the directory move stdout = %q", out)
	}
	release()
	if got := column(ok(t, s.tg, "ls", "/locked/y")); strings.Join(got, " ") != "x f.txt" {
		t.Errorf("ls /locked/y after the moves = %v", got)
	}
}

// schemaDown is the last step: status shows both sets at head; down
// removes every object table and keeps the history tables; reset without
// --yes is refused before touching the database; reset --yes drops the
// history tables too; a reset over a database with nothing applied
// succeeds; and up replays from zero, so the tree is empty again.
func (s *script) schemaDown(t *testing.T) {
	s.statusAtHead(t)
	out := ok(t, s.tg, "schema", "down")
	if !strings.HasPrefix(out, "schema down:") {
		t.Errorf("schema down stdout = %q, want the result line", out)
	}
	for _, table := range objectTables {
		if livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("after schema down, table %s still exists", table)
		}
	}
	for _, table := range historyTables {
		if !livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("after schema down, history table %s is gone", table)
		}
	}
	out = ok(t, s.tg, "schema", "status")
	if got := lines(out); len(got) != 3 || !strings.Contains(got[1], "2 file") || !strings.Contains(got[2], "2 bookmark") {
		t.Errorf("schema status after down:\n%s", out)
	}

	refused(t, s.tg, "--yes", "schema", "reset")
	for _, table := range historyTables {
		if !livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("schema reset without --yes dropped history table %s", table)
		}
	}
	out = ok(t, s.tg, "schema", "reset", "--yes")
	if !strings.HasPrefix(out, "schema reset:") {
		t.Errorf("schema reset --yes stdout = %q, want the result line", out)
	}
	for _, table := range historyTables {
		if livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("after schema reset --yes, history table %s still exists", table)
		}
	}
	// A reset over a database with nothing applied succeeds, and up
	// replays from zero.
	ok(t, s.tg, "schema", "reset", "--yes")
	ok(t, s.tg, "schema", "up")
	for _, table := range objectTables {
		if !livetest.Exists(s.ctx, t, s.db, table) {
			t.Errorf("after the replay, table %s is missing", table)
		}
	}
	s.statusAtHead(t)
	if got := column(ok(t, s.tg, "ls", "/")); len(got) != 0 {
		t.Errorf("ls / after the replay = %v, want an empty tree", got)
	}
}

// TestIsolation is the proof that isolation is configuration: two
// configurations of the binary, A and B, each with its own database and
// container, build trees with overlapping paths, and neither sees the
// other. ls in A does not show B's entries and vice versa; cat of the
// same path returns each configuration's own bytes; a unit's directory
// and bookmark in A do not appear in B; each database holds exactly its
// own file rows and each container exactly its own objects; rm -r in A
// leaves B intact; schema reset in A leaves B's tables and rows; and
// after both are torn down neither database nor container remains.
func TestIsolation(t *testing.T) {
	ctx := context.Background()
	var databases, containers []string
	// Registered first, so it runs after the configurations' own cleanups
	// dropped the databases and deleted the containers.
	t.Cleanup(func() {
		for _, name := range databases {
			if livetest.DatabaseExists(ctx, t, name) {
				t.Errorf("after the teardown, database %s still exists", name)
			}
		}
		for _, name := range containers {
			if _, exists := livetest.Blobs(ctx, t, name); exists {
				t.Errorf("after the teardown, container %s still exists", name)
			}
		}
	})
	dbA, a := open(t, "", "A")
	dbB, b := open(t, "", "B")
	databases = append(databases, livetest.DatabaseName(t, a.dsn), livetest.DatabaseName(t, b.dsn))
	containers = append(containers, a.container, b.container)
	if a.container == b.container || a.dsn == b.dsn {
		t.Fatalf("the two configurations share a database or a container")
	}
	unit := blobfs.NewID()

	// Both trees hold /docs/a.txt, with different bytes. A also holds a
	// unit's directory with a file, and the unit's bookmark.
	for _, tg := range []target{a, b} {
		ok(t, tg, "schema", "up")
		ok(t, tg, "mkdir", "/docs")
	}
	putContent(t, a, "alpha\n", "/docs/a.txt")
	putContent(t, b, "bravo\n", "/docs/a.txt")
	ok(t, a, "mkdir", "/only-a", "--unit", unit)
	putContent(t, a, "secret\n", "/only-a/secret.txt")
	ok(t, a, "bookmark", "add", "/docs/a.txt", "--unit", unit, "--active")

	// Listings and reads see their own configuration only.
	if got := column(ok(t, a, "ls", "/")); strings.Join(got, " ") != "docs only-a" {
		t.Errorf("ls / in A = %v", got)
	}
	if got := column(ok(t, b, "ls", "/")); strings.Join(got, " ") != "docs" {
		t.Errorf("ls / in B = %v", got)
	}
	if out := ok(t, a, "cat", "/docs/a.txt"); out != "alpha\n" {
		t.Errorf("cat in A = %q", out)
	}
	if out := ok(t, b, "cat", "/docs/a.txt"); out != "bravo\n" {
		t.Errorf("cat in B = %q", out)
	}
	refused(t, b, "not found", "ls", "/only-a")
	refused(t, b, "not found", "cat", "/only-a/secret.txt")
	if got := column(ok(t, a, "ls", "/", "--unit", unit)); strings.Join(got, " ") != "only-a" {
		t.Errorf("ls / as the unit in A = %v", got)
	}
	if out := ok(t, b, "ls", "/", "--unit", unit); len(column(out)) != 0 || !strings.Contains(out, "total 0") {
		t.Errorf("ls / as the unit in B:\n%s", out)
	}
	if got := bookmarkPaths(ok(t, a, "bookmark", "ls", "--unit", unit)); strings.Join(got, " ") != "/docs/a.txt" {
		t.Errorf("bookmark ls in A = %v", got)
	}
	if out := ok(t, b, "bookmark", "ls", "--unit", unit); len(bookmarkPaths(out)) != 0 || !strings.Contains(out, "total 0") {
		t.Errorf("bookmark ls in B:\n%s", out)
	}
	refused(t, b, "has no bookmark of the file", "bookmark", "rm", "/docs/a.txt", "--unit", unit)

	// Each database holds exactly its own rows, and each container exactly
	// its own objects, under the keys stat reports.
	keyA, keySecret := field(ok(t, a, "stat", "/docs/a.txt"), "key"), field(ok(t, a, "stat", "/only-a/secret.txt"), "key")
	keyB := field(ok(t, b, "stat", "/docs/a.txt"), "key")
	if keyA == keyB || keyA == "" || keyB == "" {
		t.Errorf("the keys of the two a.txt files: A %q, B %q", keyA, keyB)
	}
	const fileNames = "SELECT name FROM blobfs_file ORDER BY name"
	if got := strings1(ctx, t, dbA, fileNames); strings.Join(got, " ") != "a.txt secret.txt" {
		t.Errorf("A's blobfs_file rows = %v", got)
	}
	if got := strings1(ctx, t, dbB, fileNames); strings.Join(got, " ") != "a.txt" {
		t.Errorf("B's blobfs_file rows = %v", got)
	}
	if n := count(ctx, t, dbA, "SELECT COUNT(*) FROM bookmark"); n != 1 {
		t.Errorf("A holds %d bookmark rows, want 1", n)
	}
	for _, table := range []string{"bookmark", "directory_owner"} {
		if n := count(ctx, t, dbB, "SELECT COUNT(*) FROM "+table); n != 0 {
			t.Errorf("B holds %d %s rows, want none", n, table)
		}
	}
	blobsA, _ := livetest.Blobs(ctx, t, a.container)
	blobsB, _ := livetest.Blobs(ctx, t, b.container)
	t.Logf("[A] container %s holds %v", a.container, blobsA)
	t.Logf("[B] container %s holds %v", b.container, blobsB)
	if len(blobsA) != 2 || !slices.Contains(blobsA, keyA) || !slices.Contains(blobsA, keySecret) {
		t.Errorf("A's container holds %v, want %s and %s", blobsA, keyA, keySecret)
	}
	if len(blobsB) != 1 || blobsB[0] != keyB {
		t.Errorf("B's container holds %v, want %s", blobsB, keyB)
	}

	// rm -r in A removes A's rows and objects and nothing of B's.
	ok(t, a, "bookmark", "rm", "/docs/a.txt", "--unit", unit)
	ok(t, a, "rm", "-r", "/docs")
	refused(t, a, "not found", "cat", "/docs/a.txt")
	if out := ok(t, b, "cat", "/docs/a.txt"); out != "bravo\n" {
		t.Errorf("cat in B after A's rm -r = %q", out)
	}
	if got := column(ok(t, b, "ls", "/docs")); strings.Join(got, " ") != "a.txt" {
		t.Errorf("ls /docs in B after A's rm -r = %v", got)
	}
	if got := strings1(ctx, t, dbA, fileNames); strings.Join(got, " ") != "secret.txt" {
		t.Errorf("A's blobfs_file rows after rm -r = %v", got)
	}
	if got := strings1(ctx, t, dbB, fileNames); strings.Join(got, " ") != "a.txt" {
		t.Errorf("B's blobfs_file rows after A's rm -r = %v", got)
	}
	blobsA, _ = livetest.Blobs(ctx, t, a.container)
	blobsB, _ = livetest.Blobs(ctx, t, b.container)
	if len(blobsA) != 1 || blobsA[0] != keySecret {
		t.Errorf("A's container after rm -r holds %v, want %s", blobsA, keySecret)
	}
	if len(blobsB) != 1 || blobsB[0] != keyB {
		t.Errorf("B's container after A's rm -r holds %v, want %s", blobsB, keyB)
	}

	// schema reset in A drops A's tables and leaves B's, with its row.
	ok(t, a, "schema", "reset", "--yes")
	for _, table := range objectTables {
		if livetest.Exists(ctx, t, dbA, table) {
			t.Errorf("after A's reset, A's table %s still exists", table)
		}
		if !livetest.Exists(ctx, t, dbB, table) {
			t.Errorf("after A's reset, B's table %s is gone", table)
		}
	}
	if out := ok(t, b, "cat", "/docs/a.txt"); out != "bravo\n" {
		t.Errorf("cat in B after A's reset = %q", out)
	}
	ok(t, b, "rm", "-r", "/docs")
	ok(t, b, "schema", "reset", "--yes")
	if blobs, _ := livetest.Blobs(ctx, t, b.container); len(blobs) != 0 {
		t.Errorf("B's container after its rm -r holds %v", blobs)
	}
}
