// Package files is the consumer's file-system layer over blobfs: the one
// domain layer of the command-line file system, over the one root the
// schema seeds. It owns the directory_owner and bookmark tables the
// consumer's migration set creates, the consumer's read models over them,
// the adapter over the object store, and the mkdir, ls, put, cat, stat,
// mv, rm, rmdir, and bookmark commands. The consumer uses blobfs.Directory and
// blobfs.File as the library defines them and does not restate them,
// except in the read models, where the scanner's rules force it to.
//
// The package has one file per role:
//
//   - entities.go holds the row type of the consumer's owner table, the
//     scan types of its two read models, and the request and result
//     shapes the commands build.
//   - errors.go holds the errors the consumer adds to blobfs's and the
//     names of the bookmark table's constraints, which database.go maps to
//     them.
//   - statements/ holds the consumer's SQL: the owner row's insert and
//     read and removal, the owned_directories projection base, the
//     bookmark's insert, delete, and per-file count, and the bookmarks
//     projection base. The owner projection
//     includes blobfs's published column list and joins directory_owner,
//     so ls / --unit lists a unit's own top-level directories. The
//     bookmark projection joins bookmark to blobfs_file and computes each
//     row's path by a recursion correlated on the file's directory, so
//     bookmark ls lists a unit's bookmarks with their full paths at a cost
//     proportional to the unit's bookmark count times the depth.
//   - database.go is the only file that imports sqlate/query. It builds the
//     program's one pattern catalog, compiles the consumer's statements
//     against it, binds the typed handles, converts a Listing to the
//     library's listing and to the query library's directives, maps the
//     bookmark table's constraint violations to the consumer's sentinels
//     (one table for the insert, one for the file delete the foreign key
//     refuses), takes the option that chooses blobfs's variant, and
//     verifies every statement against the database.
//   - storage.go is the only application file that imports go-storage and
//     its Azure Blob provider. It adapts the storage store to the object
//     operations the file commands make, implements blobfs's key
//     validation over the provider's rules, maps the store's errors onto
//     the domain's, and opens and starts the store from the environment
//     for the composition root.
//   - blobfs.go composes Mkdir, List, Put, Stat, Open, Move, Remove,
//     RemoveDirectory, RemoveTree, AddBookmark, RemoveBookmark, and
//     ListBookmarks from the library's methods, the consumer's statements,
//     and the object store. List and ListBookmarks each run in one
//     read-only repeatable-read transaction. Put is the two-phase write:
//     the pending row in a transaction of its own, the object write, and
//     the completion on the pool. Remove is its mirror: the bookmark check
//     and the begin in a transaction of its own, the object delete, and
//     the row's removal on the pool. Move resolves both paths and runs the
//     library's move in one transaction, under the tree lock for a
//     directory, and keeps every move under one top-level directory.
//     RemoveDirectory removes the owner row and the directory in one
//     transaction, and RemoveTree walks a tree through those two, children
//     first. AddBookmark resolves the file and inserts the bookmark in one
//     transaction.
//   - commands.go builds the commands over a Store constructor and renders
//     through output.
//
// The package imports no admin package.
//
// The ownership rehearsal has two grains. At the directory grain an owner
// row binds a top-level directory to a unit, and a listing under --unit
// checks that row once, at the top-level ancestor of the listed path,
// never per row. At the file grain a bookmark row binds a file to a unit,
// at most one of a unit's bookmarks is active under a partial unique
// index, and the bookmark listing is the consumer-anchored read model:
// a projection over the consumer's table joined to the library's, with
// the path computed per row.
package files
