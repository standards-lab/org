# blobfs's proposed API

Input to `blobfs.build`'s own planning, not a design ready to implement verbatim: `blobfs.build`'s
own session owns the final names, signatures, and file map. What follows is what
`blobfs.experiment` proposes, drawn from the closed, archived spike [spike-blobfs](https://github.com/JaimeStill/spike-blobfs); every claim below is verifiable
there. `blobfs.md` states what the
library is and why; this document states what it would expose.

## Shape: one compiled store, two operation handles

One constructor compiles the persistence package's statements once and returns a value carrying
two operation handles, one over `blobfs_directory` rows and one over `blobfs_file` rows:

```go
store, err := <persistence>.New(catalog, dialect, opts...)   // compiles once, chooses the engine
store.Directories   // the operations over directory rows
store.Files         // the operations over file rows
store.Verify(ctx, sess)
store.Statements()
```

The engine is chosen by an option built from an engine package's own constructor
(`data.WithVariant(postgres.New(catalog, dialect))`), never by a second constructor path; see
`blobfs.md`, "Layers and the engine package."

The calling convention holds across every operation: context and session are the first two
arguments, ids are the primary handle, a guarded step takes the version the caller already read,
and a session passes through unwrapped. Four operations are correct only inside a transaction — a
directory move, the engine's tree lock, holding a file, and beginning a file delete — and each
should say so in its own signature by taking a transaction type rather than the general session
type, so the requirement is a compile-time fact and not a runtime check.

## `Directories`

| Operation | What it does |
|-----------|--------------|
| `List(ctx, sess, parentID, listing)` | The directory's children, one page. `List` at the root id lists the depth-one directories. |
| `Find(ctx, sess, id)` | A directory by id. |
| `FindByName(ctx, sess, parentID, name)` | A directory's child by name, the directory-side twin of a file lookup by name. |
| `FindByPath(ctx, sess, startID, path)` | A directory reached by a relative path below a directory id; an empty path returns the start. An absolute-path spelling with a leading slash is a convention of a consumer's own input syntax, not a library concept. |
| `Path(ctx, sess, id)` | A directory's path, computed at read time. |
| `Create(ctx, sess, parentID, name, opts...)` | Creates a directory under its parent. A caller-supplied id is an option, for a seeded directory that must keep its id across resets. |
| `Ensure(ctx, sess, parentID, name, opts...)` | Insert-or-find: returns the directory and whether this call created it, for a seeder that runs repeatedly. |
| `Move(ctx, tx, id, parentID, name, version)` | Moves or renames a directory, guarded by the version read. |
| `Delete(ctx, sess, id)` | Removes an empty directory; the foreign keys are the guard against a non-empty one. It takes no version, since the foreign keys already refuse the case a stale version would catch. |
| `IsWithin(ctx, sess, id, ancestorID)` | Whether a directory lies within another, exported as a pure tree predicate a consumer can use for its own scope check. |
| `LockTree(ctx, tx)`, `Serializes()` | The engine's tree lock and the capability probe that reports whether the current engine serializes opposing moves. |

## `Files`

| Operation | What it does |
|-----------|--------------|
| `List(ctx, sess, directoryID, listing)` | The files in one directory, one page. |
| `Find(ctx, sess, id)` | A file by id. |
| `FindByName(ctx, sess, directoryID, name)` | A file by name within a directory. |
| `Create(ctx, sess, keys, directoryID, name, contentType, opts...)` | Inserts a file's row as pending, before any object exists, and returns the key the consumer must store under. A caller-supplied id is an option. |
| `Ensure(ctx, sess, keys, directoryID, name, contentType, opts...)` | Insert-or-find, reporting whether the call created a new row, resumed a pending one an earlier write left, or found the name already present. A put, a copy, and a seeder each decide what "already present" means for their own case. |
| `Complete(ctx, sess, id, version, obj)` | Records what the consumer's object store reported and moves the row to available. |
| `Hold(ctx, tx, id, opts...)` | Locks the row for the rest of the caller's transaction without changing its version, the reference-then-delete rule's library half; `AtVersion` is the option for a caller acting on a version it already read. |
| `Move(ctx, sess, id, directoryID, name, version)` | Moves or renames a file. |
| `Delete(ctx, tx, id)` | Moves a file's row to deleting and returns it with its key, so the consumer deletes the object next. |
| `Purge(ctx, sess, id)` | Removes a deleting row; succeeds on a row already gone, refuses one that is not deleting. |

Not part of this list, and not recommended for `blobfs.build`: a path-based file lookup (a
consumer resolves the directory, then looks the file up by name — two calls, not a library
operation), a lookup by the file's storage key (deferred; see `blobfs.md`), and any operation over
more than one directory.

## Four questions this document resolves

**Naming the two steps of a write and of a delete.** The library never touches the object store, so
a file write is unavoidably at least two steps: a row that can exist before any bytes do, and a
step that records what the consumer's own store reported. `Create` names the first honestly — it
creates the row — and `Complete` names the second without claiming the library touched any bytes;
`Delete` and `Purge` are the same shape on the delete side. Neither type gets an `Edit` operation:
nothing outside a move (a rename) or the write and delete protocols mutates a field a consumer
would otherwise reach for `Edit` to change.

**What a lookup "by key" means.** The library's own vocabulary already reserves "key" for the
storage key, `<id>/<name>`, which nothing looks up by today. A lookup by directory and name —
`FindByName` — is what a write's resume step and path resolution's last step already need, and it
is the reading proposed here. A true lookup by storage key is deferred, since it needs a new index
this schema does not carry.

**Whether a listing is scoped to one directory or the whole tree.** Every listing proposed here
takes a directory id and lists that directory alone, matching the library's stated navigation
principle. An unscoped listing across the whole tree or a subtree is the deferred search capability
named in `blobfs.md`, not something this API adds by another name.

**Whether either type needs a generic `Edit`.** No: everything a consumer might reach for `Edit` to
change is already `Move` (a rename) or a field the write or delete protocol owns.

`Copy` was considered for both types and dropped. A file copy is a consumer composition — find the
source, create the destination row, stream the bytes through the consumer's own object store, then
complete the write — with nothing the library needs to add; a directory copy is the same shape of
walk as the deferred subtree operations.

## Shared types and the `Variant` interface

`Listing`, one page result type, a total mode, and a cursor error type are shared between the two
handles rather than duplicated per type: the field contract for what a listing accepts is each
statement's own declared header, and the library already refuses a filter or sort naming a field a
statement did not declare, so sharing the request and result types couples nothing between the two
entities. The engine's variation-point interface — the tree lock and its capability probe, the
file-delete begin, the write steps' returning-row forms, path resolution, and the keyset predicate
— stays exported, since a consumer's own engine package needs to implement it. It ships at its full
size for the first release, with the documented expectation that it shrinks once `sqlate` lands the
capabilities `sqlate-library-support.md` records; a consumer embeds a base variant regardless, so a
future removal costs an implementer nothing, only a caller that invoked the removed method
directly.

## Prerequisites and what shapes the API

One prerequisite blocks the API outright: the engine package's migration set must be the type
`sqlate` v0.2.0 exports, so `blobfs.sources` has to land before `blobfs.build` can compile its
engine package as proposed (`blobfs.md`, "Sequencing," names the fallback if it slips).

Several `sqlate` adjustments shape the API without blocking it — a base statement that binds its
own parameters, a total mode and a keyset cursor for a listing, a guard that can carry a status
predicate and return the row, verified field types, a resolved library namespace, a multi-line
native declaration, a schema-qualified history check, and a mapper that flattens embedded structs.
Each is recorded with its shape in `sqlate-library-support.md`; if they land before `blobfs.build`
starts, the listing composer and the cursor can collapse onto `sqlate`'s own projection instead of
being carried as the library's own code.

## Repository layout

```
blobfs/                     the root package: entities, status, key and name rules, sentinels
blobfs/<persistence>/       the persistence package (name open; see sqlate-library-support.md)
blobfs/<persistence>/statements/   the standard-tier statement files
blobfs/<persistence>/patterns/     the published column lists
blobfs/<persistence>/<persistence>test/   the conformance suite, beside the package it tests
blobfs/postgres/            the Postgres engine: the variant, its native statements, the DDL
blobfs/postgres/statements/, blobfs/postgres/migrations/
blobfs/internal/<testsupport>/   the throwaway-database, explain, and fixture helpers the
                            integration tier shares; internal, so it is not API
```

No sub-module: the engine package imports no driver, so there is no dependency weight to isolate.
No command ships with the library: the experiment's tool proved the library and is not itself
promoted; if an operator's command is ever wanted, it is laid out beside the library, never inside
it. The layout follows `go-web-service`'s file-splitting convention (a role file that outgrows
navigability splits by concern, the role kept as the prefix, never into sub-packages) as precedent,
not as a binding rule the architecture layer states — it has none yet for intra-package layout. The
`split-check` task moves with the library, reduced to three rules (the root imports neither
`sqlate` nor `go-storage`; the persistence package never imports `go-storage`; the engine package
imports the persistence package and `sqlate` only) plus one addition: no non-test file under the
library imports the test-support package.

## Names `blobfs.build`'s own SETTLE still owns

- The persistence package's name.
- The exact words for the second step of each protocol (`Complete`/`Purge` proposed here;
  `Commit`/`Remove` considered and set aside because "Delete then Remove" reads ambiguously).
- Whether `Directories.Delete` should take a version, given the foreign keys already guard the case
  a stale version would.
- Whether `Listing` and its page type stay shared across both handles or become one pair per
  handle.
- A directory `Rename` that skips the tree lock, if a consumer's volume ever needs it.
- Whether the engine's variation-point interface ships at its full size or is trimmed before first
  release, given the documented expectation that part of it shrinks later.
