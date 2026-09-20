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

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
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

// TestSchemaCommands runs the binary's schema up and schema down against a
// throwaway database named through BLOBFS_DSN alone: up creates the
// consumer's tables over blobfs's and seeds the root, prints its one result
// line, and exits zero; down removes every object table and exits zero.
// The directory and file commands are the next stage's scripted run.
func TestSchemaCommands(t *testing.T) {
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	bin := build(t)

	out, errOut, code := run(t, bin, dsn, "schema", "up")
	if code != 0 {
		t.Fatalf("schema up exited %d: %s", code, errOut)
	}
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

	out, errOut, code = run(t, bin, dsn, "schema", "down")
	if code != 0 {
		t.Fatalf("schema down exited %d: %s", code, errOut)
	}
	if !strings.HasPrefix(out, "schema down:") {
		t.Errorf("schema down stdout = %q, want the result line", out)
	}
	for _, table := range objectTables {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after schema down, table %s still exists", table)
		}
	}
}
