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
// consumer's tables over blobfs's, prints its one result line, and exits
// zero; down removes every object table and exits zero.
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
	for _, table := range []string{"blobfs_volume", "blobfs_directory", "blobfs_file", "volume_owner", "volume_bookmark"} {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after schema up, table %s is missing", table)
		}
	}

	unit := "0193b0a2-1111-7000-8000-000000000001"
	steps := []struct {
		args   []string
		code   int
		stdout []string // substrings expected on stdout, in order
		stderr string   // a substring expected on stderr, for a failing step
	}{
		{args: []string{"volume", "create", "docs", "--unit", unit}, stdout: []string{"volume create: docs (id ", "unit " + unit}},
		{args: []string{"volume", "create", "docs", "--unit", unit}, code: 1, stderr: "name taken"},
		{args: []string{"mkdir", "docs:/a"}, stdout: []string{"mkdir: docs:/a\n"}},
		{args: []string{"mkdir", "docs:/a/b"}, stdout: []string{"mkdir: docs:/a/b\n"}},
		{args: []string{"mkdir", "docs:/x/y"}, code: 1, stderr: "not found"},
		{args: []string{"ls", "docs:/a"}, stdout: []string{"KIND  NAME  SIZE  STATUS  PATH\n", "dir   b     -     -       /a/b\n", "total: 1 directories, 0 files\n"}},
		{args: []string{"ls", "docs:/a", "--unit", unit}, stdout: []string{"dir   b", "total: 1 directories, 0 files\n"}},
		{args: []string{"ls", "docs:/a", "--unit", "0193b0a2-2222-7000-8000-000000000002"}, stdout: []string{"KIND  NAME  SIZE  STATUS  PATH\n", "total: 0 directories, 0 files\n"}},
		{args: []string{"volume", "ls"}, stdout: []string{"NAME  VERSION  UNIT", "docs  1        " + unit, "total: 1\n"}},
		{args: []string{"volume", "rename", "docs", "manuals"}, stdout: []string{"volume rename: docs -> manuals (version 2)\n"}},
		{args: []string{"ls", "manuals:/a"}, stdout: []string{"dir   b     -     -       /a/b\n", "total: 1 directories, 0 files\n"}},
		{args: []string{"ls", "manuals:/"}, stdout: []string{"dir   a     -     -       /a\n", "total: 1 directories, 0 files\n"}},
		{args: []string{"ls", "docs:/a"}, code: 1, stderr: "not found"},
		{args: []string{"volume", "ls", "--sort", "name:desc", "--size", "1"}, stdout: []string{"manuals  2        " + unit, "total: 1\n"}},
		{args: []string{"ls", "manuals"}, code: 1, stderr: "has no colon"},
	}
	for _, step := range steps {
		out, errOut, code := run(t, bin, dsn, step.args...)
		if code != step.code {
			t.Errorf("%v exited %d, want %d\nstdout: %s\nstderr: %s", step.args, code, step.code, out, errOut)
			continue
		}
		rest := out
		for _, want := range step.stdout {
			i := strings.Index(rest, want)
			if i < 0 {
				t.Errorf("%v: stdout lacks %q in order:\n%s", step.args, want, out)
				break
			}
			rest = rest[i+len(want):]
		}
		if step.stderr != "" && !strings.Contains(errOut, step.stderr) {
			t.Errorf("%v: stderr = %q, want %q", step.args, errOut, step.stderr)
		}
		if step.code == 0 && errOut != "" {
			t.Errorf("%v: stderr = %q, want nothing", step.args, errOut)
		}
	}

	out, errOut, code = run(t, bin, dsn, "schema", "down")
	if code != 0 {
		t.Fatalf("schema down exited %d: %s", code, errOut)
	}
	if !strings.HasPrefix(out, "schema down:") {
		t.Errorf("schema down stdout = %q, want the result line", out)
	}
	for _, table := range []string{"blobfs_volume", "blobfs_directory", "blobfs_file", "volume_owner", "volume_bookmark"} {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after schema down, table %s still exists", table)
		}
	}
}
