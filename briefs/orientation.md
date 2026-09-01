# Standards Lab: an orientation

This brief is an orientation to an initiative I have been working on: Standards Lab, a
blueprint for a modern, agentic software architecture strategy. It covers what I am
building, why I believe it matters, how it is being built, and how to explore it in depth with
your own agent. There are no asks in this briefing beyond any feedback or questions you may have.

If you have further questions or would like to dig deeper into what has been fleshed out so far,
paste the line below into your agent of choice. It initializes an interview session over the
organization's context and source, answering your questions with cited sources.

> [!IMPORTANT]
> Validated with Claude and ChatGPT. Does not work with Gemini.

```md
launch interview: https://github.com/standards-lab/org/blob/main/interview.md
```

## The vision

The thesis is simple to state: clear architectural principles and standards, broken down into
appropriately layered boundaries, optimize the utility of agentic workflows in creating
well-structured software. The same structure that keeps a system legible to people is what lets
agents build it well.

While we are in a holding pattern waiting for our plans to materialize, I am taking the time to
wrestle with the lessons we have learned and the progress we have made, and to find the
principles and patterns that optimize the conditions for long-term success and the longevity of
the capabilities we build.

The boundaries in that thesis are concrete: the modern primitives of what software emits — the
library, the binary, the container image. Each primitive is a modular boundary: the artifact
where maintenance is contained, vulnerabilities and technical debt are mitigated, and a
capability becomes reusable. Layered appropriately, the boundaries compose into an ecosystem of
reusable components: foundations get solved once and adopted rather than re-derived project by
project, and our capacity shifts toward creating, innovating, and leading the standard forward.
They also keep ownership and change legible: lines of effort and responsibility attach to
tangible artifacts, and a small, deliberate dependency surface stays pinned and current, so we
drive change rather than react to it. A few examples of the patterns in play:

- **Interoperable, domain-driven services.** Services scoped to a domain, composing across
  clear interfaces rather than accumulating into monoliths.

- **Data composition over rigid schema.** Data modeled for composition rather than locked into
  tightly coupled schema.

- **Software architecture above the transport layer.** Our enterprise has operationalized the
  OSI model's lower layers well: building and configuring networks, VNETs, VLANs, and firewalls
  is settled practice. The higher, data-driven layers are not held to a comparable discipline,
  even though the commercial sector's standards for them are mature; this strategy establishes
  that discipline for us.

The agentic questions sit alongside the architectural ones. How do we integrate AI agents well,
and capture the principles of successful harness programming? How does the technology raise the
velocity of the enterprise rather than one person? How does the same blueprint pattern serve
other disciplines? And, if this proves successful, a longer-term question worth holding: how does
an enterprise designate its experts to cultivate a discipline's evolution and adoption strategy?

## How it is being built

The development workflow is part of the work, not tooling on the side. Harness programming
treats how agents develop software — the workflows, conventions, and skills they run — as
something to engineer with the same discipline as the software itself, and its principles are
part of the architecture this effort captures.

The concrete workflow is the agent skill marathon (from the notion that it's a marathon, not a sprint).
It is an extensible long-haul development skill built in this effort. Each session plans one concrete
step, and a maintained reset context carries continuity from one session to the next. The
written context lives alongside the code, layered by volatility, so knowledge graduates from
concept to design to principle as the work proves it. Its workspace feature coordinates
development across repositories, letting one step move the libraries, the service, and the docs
together. Extensions attach at its hook points; the first is a roadmap extension that keeps the
remaining path current as a side effect of the sessions themselves.

The entire effort runs on this workflow, dogfooding it on the very system it exists to build:
every session tunes it, and the principles it proves are captured as development evolves. In an
agentic system, context maintenance matters as much as the source code: the written context is
the specification that the agent and the developers working with it align on, the source of
truth for the design. The reset context, the design notes, and the live roadmap are all public
in the working context.

## The roadmap at a glance

The target is a holistic reference: a web service at v1.0, with every capability layer a
production service reasonably encounters complete in one running composition. Each layer is
proven end to end, and what it teaches feeds the SDKs, the infrastructure libraries, the
template that future services start from, and the underlying principles that shape the
architecture and standard. Solve a layer holistically once and it becomes a repeatable blueprint
that does not need to be solved again. The layers on the way there: data composition and CQRS,
the web handler contract, auth and access control, observability, object storage, messaging,
AI, an embedded client, and deployment.

My current focus is the data layer: the boundary between SQL and the Go that executes it. SQL
alone lacks developer-ergonomic expressiveness for dynamic composition — a template block, or a
condition rendered only when its filter is provided. This layer adds that expressiveness through
thin mechanisms in the Go library while keeping the SQL itself as native as possible. Authored
SQL files are the source of truth: accessible to DBAs, and retaining the language's full
flexibility for composing reusable query infrastructure. No strings buried in Go code; no SQL
hobbled by an ORM.

That is one layer of many, and it sets the honest framing for the whole: this is a
comprehensive review of how we can optimize the way we build production software, worked out in
running code toward the vision laid out above. It does not have all the answers yet. The point
is taking the time to find enough of the right conventions to build effectively; it is coming
together well, and dialing it in takes time.

## Explore it yourself

- [Documentation](https://github.com/standards-lab/docs) — the landing zone: the canonical home
  for the Elemental Architecture, its standards, and its principles.
  - [Elemental Architecture](https://github.com/standards-lab/docs/blob/main/architecture.md)
    — the compositional elements a program is built from and the rules that bind them,
    independent of technology.
  - [Principles](https://github.com/standards-lab/docs/blob/main/principles/index.md) — the
    architecture's principles, which every standard enhances and never loosens.
- [Harness](https://github.com/standards-lab/claude-plugins) — `claude-plugins`, the plugin
  marketplace codifying the organization's development processes: the `marathon` workflow and
  its `marathon-roadmap` extension.
- [Go Elemental](https://github.com/standards-lab/docs/blob/main/standards/go-elemental/index.md)
  — the Go implementation of the Elemental Architecture on the standard library, the
  organization's first standard.
  - [`go-core`](https://github.com/standards-lab/go-core) — the Core SDK: layered
    configuration, the process lifecycle, and the logger.
  - [`go-database`](https://github.com/standards-lab/go-database) — the SQL infrastructure
    library, with the PostgreSQL provider as a sub-module.
  - [`go-web-sdk`](https://github.com/standards-lab/go-web-sdk) — the Application SDK for web
    services.
  - [`go-web-sdk-template`](https://github.com/standards-lab/go-web-sdk-template) — scaffolds
    an initial Go Elemental web service with `gonew`.
  - [`go-web-service`](https://github.com/standards-lab/go-web-service) — the holistic
    reference web service, grown in documented layers; versionless until its 1.0.
