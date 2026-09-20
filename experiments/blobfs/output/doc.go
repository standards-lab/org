// Package output renders a command's result. Every command family renders
// through one Output the composition root builds over the process's two
// streams, so no command names a stream itself. Line writes a one-line
// success to stdout, so a command that changes state and returns nothing
// is never silent. Error writes a failure to stderr as the error's message.
// The exit code is the composition root's concern: a command returns its
// error, and the root renders it through Error and exits non-zero.
//
// The package imports no domain, admin, or library package. The stages
// that add the file commands extend it with the row rendering ls and stat
// share.
package output
