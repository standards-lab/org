// Package files is the consumer's file-system layer over blobfs: the one
// domain layer of the command-line file system, over the one root the
// schema seeds. It owns the directory_owner and bookmark tables the
// consumer's migration set creates, the consumer's read models over them,
// the adapter over the object store, and the mkdir, ls, put, cat, stat,
// cp, mv, rm, rmdir, and bookmark commands. The consumer uses blobfs.Directory and
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
//     bookmark's insert, delete, and per-file count, and the two bookmark
//     projection bases. The owner projection includes blobfs's published
//     column list and joins directory_owner, so ls / --unit lists a
//     unit's own top-level directories. The bookmarks projection joins
//     bookmark to blobfs_file and carries the file's id and directory id,
//     the handles a caller acts with, and no path; bookmarks_with_paths
//     adds each row's path, computed by a recursion correlated on the
//     file's directory at a cost proportional to the unit's bookmark
//     count times the depth, and runs only when a listing asks for paths,
//     as bookmark ls does.
//   - database.go and the database_<concern>.go files are the only files
//     that import sqlate/query. database.go builds the program's one
//     pattern catalog, compiles the consumer's statements against it,
//     binds the typed handles, converts a Listing to the library's listing
//     and to the query library's directives, takes the option that chooses
//     blobfs's variant, and verifies every statement against the database.
//     database_bookmarks.go holds the bookmark statements, maps the
//     bookmark table's constraint violations to the consumer's sentinels
//     (one table for the insert, one for the file delete the foreign key
//     refuses), and holds the bookmark operations. database_owners.go
//     holds the owner statements and operations.
//   - storage.go is the only application file that imports go-storage and
//     its Azure Blob provider. It adapts the storage store to the object
//     operations the file commands make, implements blobfs's key
//     validation over the provider's rules, maps the store's errors onto
//     the domain's, and opens and starts the store from the environment
//     for the composition root. A service composes the object store
//     through NewStorage instead, from its own configuration.
//   - blobfs.go and the blobfs_<concern>.go files (read, write, move,
//     delete, and scope) compose the operations from the library's
//     methods, the consumer's statements, and the object store. Ids are
//     the primary handle: ListDirectory, StatFile, StatDirectory,
//     OpenFile, PutFile, CopyFile, MoveEntry, and RemoveFile take
//     directory and file ids, and the path forms List, Stat, Resolve,
//     Open, Put, Copy, Move, and Remove resolve their paths and then run
//     the same steps, so a caller that holds an id from a listing acts
//     without a resolution. Mkdir,
//     RemoveDirectory, RemoveTree, AddBookmark, RemoveBookmark, and
//     ListBookmarks take paths or a unit. List, ListDirectory, and
//     ListBookmarks each run in one read-only repeatable-read
//     transaction. Put and PutFile are the two-phase write: the pending
//     row in a transaction of its own, the object write, and the
//     completion on the pool. Copy and CopyFile run the same write over
//     an available file's object, streamed from the store and back under
//     the new row's key, with the source's content type; the copy's size
//     and entity tag are what the store reports for the new object, and
//     a bookmark or an owner row does not follow a copy. Remove and RemoveFile
//     are its mirror: the begin and then the bookmark check in a
//     transaction of its own, the object delete, and the row's removal on
//     the pool; RemoveFile takes the version the caller read and holds the
//     row at it first. Move and MoveEntry run the library's move in one
//     transaction, under the tree lock for a directory, and keep every
//     move under one top-level directory. RemoveDirectory removes the
//     owner row and the directory in one transaction, and RemoveTree
//     walks a tree through those two, children first. AddBookmark
//     resolves the file, holds it through the library's HoldFile, and
//     inserts the bookmark in one transaction, so it and Remove serialize
//     on the file's row. InScope is the ownership check by id, which the
//     id-keyed operations run when they are given a Scope.
//   - commands.go builds the commands over a Store constructor and renders
//     through output. The commands call the path forms, and ls, stat,
//     cat, rm, put, cp, and mv call the id forms when an argument is
//     written as id:<uuid>; ls lists each row's id, and stat prints a
//     directory's row when no file is at the path or has the id.
//
// The package imports no admin package.
//
// The ownership rehearsal has two grains. At the directory grain an owner
// row binds a top-level directory to a unit. A listing under --unit
// derives the scope from the path and checks that row once, at the
// top-level ancestor of the listed path, never per row. An id-keyed
// operation takes the scope from the caller as a Scope, the unit and the
// directory it names as its scope root, and checks it by reading the
// scope root's owner row and then running the library's IsWithin from the
// target to the scope root; the named scope is an input to check, never
// a fact to trust. At the file grain a bookmark row binds a file to a
// unit, at most one of a unit's bookmarks is active under a partial
// unique index, and the bookmark listing is the consumer-anchored read
// model: a projection over the consumer's table joined to the library's,
// carrying the file's ids, with the path computed per row only when the
// listing asks for it.
package files
