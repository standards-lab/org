// Package drivertest is the one scripted database/sql driver the experiment's
// hermetic tests share: a connector whose connections consume scripted
// responses in call order and record every statement, prepare, and
// transaction call they receive. It supports prepare, so prepare-based
// verification has a harness, and it accepts any argument value unconverted,
// so tests see the exact Go values a runner bound. The promotion target is
// go-database's internal/drivertest, replacing the fakes duplicated across
// its packages.
package drivertest
