// Package volume is the consumer's file-system layer over blobfs: the one
// domain layer of the command-line file system. It owns the two tables the
// consumer's migration set creates, volume_owner and volume_bookmark, and
// in the later stages the file commands that exercise the library.
//
// This stage holds entities.go: the row types of the consumer's own tables.
// The consumer uses blobfs.Volume, blobfs.Directory, and blobfs.File as the
// library defines them and does not restate them. The stages that follow
// add the package's statements directory and database.go, the sole importer
// of sqlate/query; blobfs.go, the translation file over the library under
// test and the sole importer of lib/blobfs and lib/blobfs/data; storage.go,
// the sole importer of go-storage; and commands.go, the domain's cobra
// subtree. The package imports no admin package.
package volume
