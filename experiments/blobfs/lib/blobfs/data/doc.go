// Package data is the second layer of blobfs: the persistence over the
// root package's entities. It holds blobfs's own SQL statements, the
// pattern namespace it publishes, the listing composer, and the Store,
// whose methods run the statements against a sqlate.Session the caller
// provides. It is the standard-tier baseline: every statement and every
// pattern is standard SQL, so the package runs on any engine sqlate has a
// dialect for. Ids are minted in Go with blobfs.NewID, or supplied by the
// caller through WithID so a seeded row keeps its id across resets; a
// row's defaults are read back after an insert instead of returned by
// it, and paging is OFFSET and FETCH NEXT.
//
// A consumer builds one pattern catalog for its whole program, with
// query.Patterns() and Patterns() registered beside its own sources, and
// passes it to New, which compiles the embedded statements against it. The
// published patterns are the column lists the entity types scan,
// blobfs.directory_columns and blobfs.file_columns, which a consumer's own
// statements include with {{> blobfs.name}}. Every pattern is
// parameter-free, so a projection base that includes one binds no
// parameters of its own.
//
// The tree has one root, seeded by the schema with blobfs.RootID. Root,
// Directory, Mkdir, EnsureDirectory, ResolveDirectory,
// ResolveDirectoryFrom, and DirectoryPath read and write directories;
// EnsureDirectory is the insert-or-find a seeder runs, which looks the
// name up first and inserts only when it found no row. Ids are the
// primary handle: every operation takes a directory or file by id, and a
// path is an entry point. ResolveDirectory resolves an absolute path from
// the root, and ResolveDirectoryFrom resolves a relative path below a
// directory the caller holds by id, so a consumer that has an id walks
// from there instead of from the root. ListFiles and Children are the
// listings: each is an authored statement anchored on one directory, with
// the caller's filters, sort, and page composed onto it in Go from the
// query library's clause patterns, and with the total computed in the same
// statement by COUNT(*) OVER () when the Listing asks for one. A page is reached by its
// number or continued from the keyset cursor of an earlier page, and both
// walk the same order; every page reports whether rows remain after it,
// with or without a total. No operation walks the whole tree; a path is
// resolved one segment per round trip and computed by one upward walk.
// File and FileByName read one file row, by id or by directory and name.
//
// A file write is two steps around the object write the package never
// makes. BeginFileWrite validates the name, mints the id, builds the key
// and validates it against the caller's blobfs.KeyValidator, and inserts
// the row as pending; it runs on the pool or inside the caller's
// transaction, beside the caller's own rows. The caller stores the object
// under the row's key and then calls CompleteFileWrite with the row's id
// and version and what the store reported, which moves the row to
// available under the query library's version guard. A stop between the
// steps leaves the row pending, where a listing finds it and a retry of
// the same write, having found it through FileByName, completes it.
// BeginOrResumeFileWrite is the begin step and that lookup as one
// operation: it returns the row that holds the name and a WriteOutcome
// that says whether the row was created, resumed as pending, or found in
// another status, so a put, a copy, and a seeder share one write
// protocol and each decides what a found row means. There is no fail
// step: an abandoned write is removed through the delete steps, which a
// pending row already allows.
//
// A file delete is two steps around the object delete the package never
// makes. BeginFileDelete moves the row to deleting and returns it, so the
// caller deletes the object under the row's key; CompleteFileDelete then
// removes the row, and only a deleting row. The begin is idempotent on a
// row already deleting and the complete succeeds on a row already gone,
// so a retry after a stop at any step finishes the delete. The deleting
// status is the durable marker that makes the retry possible once the
// object is gone: without it a row whose object was removed would read
// as available. RemoveDirectory removes one empty directory; there is no
// cascade and no recursive delete, and a consumer that wants one walks
// the tree, files and then directories, deepest first.
//
// A consumer's row that references a file, such as a bookmark, follows
// the reference-then-delete rule: the consumer calls HoldFile inside the
// transaction that inserts the row, before the insert, and a file delete
// runs its begin step before it reads the consumer's table, in one
// transaction. HoldFile locks the file's row without changing it, and it
// refuses a deleting file; the begin step takes the same lock. So the two
// transactions serialize on the row: an insert that holds first commits
// its reference before the delete reads the table and refuses, and a
// delete that begins first makes the hold refuse. The rule is standard
// SQL, so it holds on every engine and through every variant's begin.
//
// Two operations are variation points, where an engine may do better than
// standard SQL: the tree lock that serializes directory moves, and the
// first step of a file delete. The Variant interface names them, Standard
// is the baseline every engine runs (a no-op lock that reports it does
// not serialize, and the delete begin as an update and a read-back in the
// caller's transaction), and New takes another implementation through
// WithVariant: the postgres package's for Postgres, or a consumer's own,
// which may embed either and override one method. The Store forwards
// LockTree, Serializes, and BeginFileDelete to the variant and runs
// everything else from its own statements. The datatest package holds the
// conformance suite a variant must pass.
//
// Every method takes the session as an argument and passes it through
// unwrapped, so a call runs against the pool or inside the caller's
// transaction, and a statement headed transaction: required (the
// baseline's delete begin, the directory move's update, and the hold)
// refuses a session that is not a *sqlate.Tx. A violation of one of
// blobfs's own constraints becomes blobfs.ErrNameTaken,
// blobfs.ErrIDTaken, blobfs.ErrNotFound, blobfs.ErrRootDirectory, or, on
// a delete, blobfs.ErrNotEmpty. A
// violation of a constraint blobfs does not own returns unclassified on a
// write, wrapped with the operation's context; on a delete a foreign key
// blobfs does not own is a consumer's row that references the one being
// removed, which the package reports as blobfs.ErrReferenced by the
// violation's class, with the constraint's name reachable for the
// consumer to match.
package data
