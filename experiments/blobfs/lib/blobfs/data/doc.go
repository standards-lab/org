// Package data is the second layer of blobfs: the persistence over the
// root package's entities. It holds blobfs's own SQL statements, the
// pattern namespace it publishes, the listing composer, and the Store,
// whose methods run the statements against a sqlate.Session the caller
// provides. It is the standard-tier baseline: every statement and every
// pattern is standard SQL, so the package runs on any engine sqlate has a
// dialect for. Ids are minted in Go with blobfs.NewID, a row's defaults are
// read back after an insert instead of returned by it, and paging is
// OFFSET and FETCH NEXT.
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
// Directory, Mkdir, ResolveDirectory, and DirectoryPath read and write
// directories. ListFiles and Children are the listings: each is an
// authored statement anchored on one directory, with the caller's filters,
// sort, and page composed onto it in Go from the query library's clause
// patterns, and with the total computed in the same statement by
// COUNT(*) OVER () when the Listing asks for one. A page is reached by its
// number or continued from the keyset cursor of an earlier page, and both
// walk the same order. No operation walks the whole tree; a path is
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
// the same write, having found it through FileByName, completes it. There
// is no fail step: an abandoned write is removed through the delete steps,
// which a pending row already allows.
//
// Two operations are variation points, where an engine may do better than
// standard SQL: the tree lock that serializes directory moves, and the
// first step of a file delete. The Variant interface names them, Standard
// is the baseline every engine runs (a no-op lock that reports it does
// not serialize, and the delete begin as an update and a read-back in the
// caller's transaction), and New takes another implementation through
// WithVariant: the pgnative package's for Postgres, or a consumer's own,
// which may embed either and override one method. The Store forwards
// LockTree, Serializes, and BeginFileDelete to the variant and runs
// everything else from its own statements. The datatest package holds the
// conformance suite a variant must pass.
//
// Every method takes the session as an argument and passes it through
// unwrapped, so a call runs against the pool or inside the caller's
// transaction, and a statement headed transaction: required refuses a
// session that is not a *sqlate.Tx. A violation of one of blobfs's own
// constraints becomes blobfs.ErrNameTaken, blobfs.ErrNotFound, or
// blobfs.ErrRootDirectory; a violation of a constraint blobfs does not own
// returns unclassified, wrapped with the operation's context.
package data
