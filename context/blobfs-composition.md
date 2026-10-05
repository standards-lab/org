# Composing `blobfs` into a consumer

What go-web-service's storage layer proved about composing `blobfs` that neither blobfs's guide
(`blobfs/docs/`) nor go-web-service's code and `context/domain-architecture.md` state yet: the
write, delete, and move protocols as a consumer sequences them, whose complete step is the
transaction an event is enqueued in (`messaging.md`, "Outbox sequencing"), and the operating
constraints. The composition root,
ownership, seeding, and set registration are expressed in go-web-service and are not restated.
This note goes once blobfs's guide carries these sections.

## The protocols as the service sequences them

**The upload.** In one transaction: check the scope and begin the file's write, and nothing else —
no consumer row may reference the pending file, since a reference refuses the library's stale
reclaim on every pass and strands the row (the logo's first write did exactly this, found by a
layering review). Outside any transaction: upload under the row's key. On the pool: complete the
write. A put or completion that fails abandons the row through the delete steps, on a context that
outlives the request's cancellation; a completion refused because a sweep reached the row first
deletes the object just stored (the writer rule). A consumer's reference to the file, such as the
logo's join row, is written after completion, in the transaction that holds the file. The service
stages these steps once, as shared protocols in its `data` package (write, ensure, retire, purge,
serve), bound for the library.

**The delete.** In one transaction: hold the file (the reference-then-delete rule) if any of the
consumer's own rows are about to reference it, or check the consumer's own referencing rows and
refuse while any exist, then begin the library's delete. Outside any transaction: delete the object.
On the pool: purge the row. The consumer's own foreign key into the library's file table is
the backstop for a reference that slips in after the check, and the consumer maps that constraint's
name to its own sentinel at the purge step. A recursive removal is the library's mark and sweep: the
request marks the branch deleting at the version it read and answers 202, and the sweep, run as a
background worker woken by the request, on an interval, and once at startup, removes the branch's
objects and rows and reclaims stale pending and deleting rows past an age. The sweep's refusals are
logged and retried, never fatal.

**The move.** Resolve both paths and run the library's move in one transaction; a directory move
takes the engine's lock through the variant. On an engine with no native lock, the consumer
serializes directory moves itself — one mover per process, or every moving transaction at
serializable isolation with a retry on the driver's serialization-failure class.

## Operating constraints

- **A correlated path recursion crosses a JIT threshold at volume.** A consumer whose read model
  computes a path per row through a correlated recursion should expect the planner's JIT compiler
  to trigger once a unit's row count crosses roughly a hundred and forty, which costs more than the
  walk itself; sorting by an indexed key instead of relying on the recursion's own order, or
  lowering the JIT threshold for the session, avoids it. The evidence section of
  [spike-blobfs](https://github.com/JaimeStill/spike-blobfs)'s `REVIEW.md` has the measured
  numbers.
- **An exact total reads the whole directory.** The window count that produces an exact total costs
  in proportion to the directory's own size, never the whole tree, but a directory with tens of
  thousands of files should read its total once, on the first page, and walk the rest by cursor with
  `query.TotalNone`.
- **The baseline engine has no tree lock.** A consumer on an engine without a native one must
  serialize directory moves itself, as above.
- **Schema changes and the sweep deadlock.** Reverting a consumer migration that references the
  library's directory table locks the two tables in the opposite order from a sweep's cascading
  removal, and Postgres aborts one side. The service quiesces the sweep around its schema-changing
  admin verbs with a per-process gate; a multi-replica reset would need a database lock the sweep
  also takes.
