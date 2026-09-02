// Package sqlheader reads the directive header of an authored SQL file: the
// run of "--" comment lines, blank lines allowed, before the first SQL token.
// A comment line of the form "-- key: value" is a directive; any other
// comment line in the header is prose and is skipped. The package knows no
// keys. Each consumer — query for tier, native, transaction, key, and field;
// migrate for transaction — decides which keys it accepts and what their
// values mean.
package sqlheader
