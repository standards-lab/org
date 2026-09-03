// Command sqlint checks the authored SQL files of the module against the
// conventions the loader cannot, or should not, enforce at runtime: every
// file under a sql/ directory loads (the header grammar, the parameter
// syntax, the field contract), a file is named for its operation and not
// its SQL verb, the parameter delimiter does not appear inside a comment
// or a string literal, a standard-tier file uses no engine-native form the
// lint knows, and a migration headed "transaction: none" holds one
// statement. It walks the working tree and reports file:line: message,
// exiting 1 on any finding. The promotion target is the harness's
// checkable rules.
package main
