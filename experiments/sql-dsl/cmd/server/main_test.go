package main

import (
	"strings"
	"testing"

	"github.com/standards-lab/go-core/process"
)

// The dispatch paths exit before any configuration or database work, so
// they are testable without either. The verbs themselves are proven against
// the live compose database and recorded in NOTES.md.

func TestRun_UnknownArgIsUsageError(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"bogus"}, &stdout, &stderr); code != process.ExitUsage {
		t.Errorf("run(bogus) = %d, want ExitUsage", code)
	}
	if !strings.Contains(stderr.String(), "usage: server") {
		t.Errorf("stderr = %q, want the usage block", stderr.String())
	}
}

func TestRun_SchemaWithoutVerbIsUsageError(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"-schema"}, &stdout, &stderr); code != process.ExitUsage {
		t.Errorf("run(-schema) = %d, want ExitUsage", code)
	}
	if !strings.Contains(stderr.String(), "verbs:") {
		t.Errorf("stderr = %q, want the schema usage block", stderr.String())
	}
}

func TestRun_SchemaUnknownVerbIsUsageError(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"-schema", "explode"}, &stdout, &stderr); code != process.ExitUsage {
		t.Errorf("run(-schema explode) = %d, want ExitUsage", code)
	}
	if !strings.Contains(stderr.String(), `unknown verb "explode"`) {
		t.Errorf("stderr = %q, want the unknown-verb message", stderr.String())
	}
}

func TestRun_SchemaHelpPrintsUsageToStdout(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run([]string{"-schema", "help"}, &stdout, &stderr); code != process.ExitUsage {
		t.Errorf("run(-schema help) = %d, want ExitUsage", code)
	}
	if !strings.Contains(stdout.String(), "usage: server -schema") {
		t.Errorf("stdout = %q, want the schema usage block", stdout.String())
	}
}
