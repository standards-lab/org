# External providers and the cross-standard port

Captured 2026-08-19 in the go-database session. A provider for a service is not bound to the
infrastructure library's repository: the contract between a provider and the base is the base
package's interface and constructor (`database.Dialect` and `database.New` for SQL), and any
module can implement it. `go-database` records this in its design: a provider is defined where
it is owned and maintained, as a sub-module of the infrastructure library or as its own library
(`go-database-mssql`, for instance).

That changes what the second provider is for and where it lives. Rather than building a second SQL
provider inside `go-database` to prove the provider contracts, the long-term plan is a port of the
`go-elemental` reference service to another engine, with that engine's provider defined as a library
within the standard or focused reference that owns the port, alongside `dotnet-minimal` as the
derived standard. The port demonstrates adoption across standards (`../design/standards.md`):
the porting standard takes `go-database`'s base module as a pinned dependency and authors only what
is specific to itself, its provider.

## What this touches

- `design/service-organization.md` says a second provider is proven elsewhere in its own
  session. Under this concept the second provider is an external library built by the port, and
  the infrastructure library gains a nested provider only when this standard answers for the
  engine. The note is revised when the plan settles.
- The roadmap re-plan (restructure step 7) decides where the port sits in the sequence and which
  engine it targets; SQL Server is the engine the enterprise precedent points at.
- `go-database`'s `Dialect` interface grows by engine difference as the port reaches it; the port is
  what stresses the interface.

## Open questions

- Whether the port is a focused reference architecture within `go-elemental` or a member of
  another standard, and how its repository is named.
- Whether an external provider's repository follows `go-<technology>-<engine>`
  (`go-database-mssql`) as a naming convention; settled when the first exists.
