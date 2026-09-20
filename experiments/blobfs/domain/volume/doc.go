// Package volume is the consumer's file-system layer over blobfs: the one
// domain layer of the command-line file system. It owns the two tables the
// consumer's migration set creates, volume_owner and volume_bookmark, the
// consumer's read models over blobfs's tables, and the volume and directory
// commands. The consumer uses blobfs.Volume, blobfs.Directory, and
// blobfs.File as the library defines them and does not restate them.
//
// The package has one file per role. entities.go holds the row types of the
// consumer's tables, the scan types of its two read models, and the request
// shapes the commands build. statements/ holds the consumer's SQL: the two
// projection bases, volume_view and file_view, which include blobfs's
// published patterns and join the consumer's volume_owner row so a listing
// carries the owning unit, and the owner statements. database.go is the
// sole importer of sqlate/query: it builds the program's one pattern
// catalog, compiles the consumer's statements against it, binds the typed
// handles, lowers a Listing to directives, and verifies everything against
// the database. blobfs.go is the translation over the library under test:
// it compiles blobfs's persistence against the same catalog and composes
// the consumer's operations from the library's methods and the consumer's
// statements. address.go parses the command line's <volume>:<path> form.
// commands.go builds the volume, mkdir, and ls commands over a Store
// constructor and renders through output. A later stage adds storage.go,
// the sole importer of go-storage, and the file commands. The package
// imports no admin package.
package volume
