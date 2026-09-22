package output

import (
	"fmt"
	"io"
)

// Output is where a command's result goes: the stream a success is written
// to and the stream a failure is written to. The composition root builds
// one and every command family renders through it. Nothing about an Output
// changes after New, so one is safe to share.
type Output struct {
	stdout, stderr io.Writer
}

// New returns an Output writing results to stdout and failures to stderr.
func New(stdout, stderr io.Writer) *Output {
	return &Output{stdout: stdout, stderr: stderr}
}

// Line writes one line to stdout: line followed by a newline.
func (o *Output) Line(line string) {
	_, _ = fmt.Fprintln(o.stdout, line)
}

// Error writes err's message and a newline to stderr.
func (o *Output) Error(err error) {
	_, _ = fmt.Fprintln(o.stderr, err.Error())
}
