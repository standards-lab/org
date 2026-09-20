// Package data is the second layer of blobfs: the persistence over the
// root package's entities. It holds blobfs's own SQL statements, the
// pattern namespace it publishes, and the Store, whose methods run the
// statements against a sqlate.Session the caller provides. It is the
// standard-tier baseline: every statement and every pattern is standard SQL,
// so the package runs on any engine sqlate has a dialect for. Ids are
// minted in Go with blobfs.NewID, a row's defaults are read back after an
// insert instead of returned by it, and paging is OFFSET and FETCH NEXT.
//
// A consumer builds one pattern catalog for its whole program, with
// query.Patterns() and Patterns() registered beside its own sources, and
// passes it to New, which compiles the embedded statements against it. The
// consumer's own base statements include the published patterns with
// {{> blobfs.name}}: blobfs.tree is the recursive directory forest with
// each directory's path inside its volume, blobfs.volume_columns,
// blobfs.directory_columns, and blobfs.file_columns are the column lists
// the entity types scan, and blobfs.directory_path and blobfs.file_path
// compose a row's path over the tree. Every pattern is parameter-free, so
// a projection base that includes one binds no parameters of its own.
//
// Every method takes the session as an argument and passes it through
// unwrapped, so a call runs against the pool or inside the caller's
// transaction, and a statement headed transaction: required refuses a
// session that is not a *sqlate.Tx. A violation of one of blobfs's own
// constraints becomes blobfs.ErrNameTaken or blobfs.ErrNotFound; a violation
// of a constraint blobfs does not own returns unclassified, wrapped with the
// operation's context.
package data
