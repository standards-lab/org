// Package query runs authored SQL files: a Source loaded from an fs.FS gives
// a domain its statements by name, each file's "--|" header declaring its
// tier, its native reach, whether it needs a transaction, and — for a
// projection base — its key and field contract. Parameters, written
// {{name}} or {{name:type}} (the type binding through CAST, verbatim),
// resolve to the dialect's positions once at load; arguments bind from Args
// by name. The engine receives the body: the header is the loader's. The
// runners are generic over a consumer-written scan function and
// take a Session, so a handle runs against the pool or inside a
// transaction alike; every error is mapped through the dialect at the
// runner boundary. Verify prepares every statement against the live schema.
// Statement text is build-time only: files under embed, never request
// input. The promotion target is go-database's query package.
package query
