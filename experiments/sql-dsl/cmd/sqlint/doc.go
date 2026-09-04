// Command sqlint checks the authored SQL files of the module against the
// conventions the loader cannot, or should not, enforce at runtime: every
// statement directory compiles against the pattern sources the runtime
// registers (the header grammar, the parameter syntax, the field
// contract, includes resolving), every pattern directory validates as a
// catalog source, a file is named for its operation and not its SQL verb,
// the parameter delimiter does not appear inside a comment or a string
// literal, a standard-tier file uses no native form the configured engine
// declares, and a migration headed "transaction: none" holds one
// statement. sqlint.toml beside go.mod configures it: a table per role
// with its directory globs and switches, an override per directory set,
// the sources and the engine as paths — a directory of the tree, or a Go
// package path resolved through go list to the version go.mod pins — each
// declaring what a consumer reads in its own [export]. It walks the
// working tree and reports file:line: message, exiting 1 on any finding.
// The promotion target is the harness's checkable rules.
package main
