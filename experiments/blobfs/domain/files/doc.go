// Package files is the consumer's file-system layer over blobfs: the one
// domain layer of the command-line file system, over the one root the
// schema seeds. It owns the directory_owner table the consumer's migration
// set creates, the consumer's read model over it, and the mkdir and ls
// commands. The consumer uses blobfs.Directory and blobfs.File as the
// library defines them and does not restate them, except in the read
// model, where the scanner's rules force it to.
//
// The package has one file per role:
//
//   - entities.go holds the row type of the consumer's table, the scan type
//     of its read model, and the request and result shapes the commands
//     build.
//   - errors.go holds the errors the consumer adds to blobfs's.
//   - statements/ holds the consumer's SQL: the owner row's insert and
//     read, and the owned_directories projection base. The projection
//     includes blobfs's published column list and joins directory_owner,
//     so ls / --unit lists a unit's own top-level directories.
//   - database.go is the only file that imports sqlate/query. It builds the
//     program's one pattern catalog, compiles the consumer's statements
//     against it, binds the typed handles, converts a Listing to the
//     library's listing and to the query library's directives, and
//     verifies every statement against the database.
//   - blobfs.go composes Mkdir and List from the library's methods and the
//     consumer's statements. List runs in one read-only repeatable-read
//     transaction.
//   - commands.go builds the mkdir and ls commands over a Store
//     constructor and renders through output.
//
// A later stage adds storage.go, the only file that imports go-storage,
// and the file commands. The package imports no admin package.
//
// The ownership rehearsal is at the directory grain: an owner row binds a
// top-level directory to a unit, and a listing under --unit checks that
// row once, at the top-level ancestor of the listed path, never per row.
package files
