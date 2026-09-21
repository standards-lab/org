// Package schema is the schema administration layer: the schema command
// family, which reports, applies, reverts, and resets the two migration
// sets the consumer's database holds.
// It is a sibling of the domain tree, never a member of it, the way the
// workspace's services keep admin/ and domain/ as separate root-level trees,
// and it imports no domain package.
//
// The package has one file per role. database.go builds the Client over the
// multi-set migrator: blobfs's set first, under its own history table, and
// the consumer's set last, under sqlate's default table, so blobfs's schema
// is at its head before the consumer's migrations reference it and a revert
// runs in reverse. commands.go builds the schema command and its status,
// up, down, and reset subcommands over a Client constructor and renders
// each result through output. The composition root opens the database and
// constructs the Client; this package receives the constructor and never
// names a DSN, a driver, or a stream.
package schema
