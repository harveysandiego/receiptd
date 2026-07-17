# 0004. Compile-time registration over runtime plugins

Status: Accepted

## Context

Receiptd needs to be extensible in several dimensions that will grow over
time: new Element types, new server-side Templates, new Provider domains
(weather, shopping, homelab), and eventually new printer transports and
output formats. The project's deployment goals — a single static Go
binary, cross-compiled for ARM64 and AMD64, Docker- and
Raspberry-Pi-friendly, minimal dependencies — constrain how that
extensibility can be implemented.

Go's standard library `plugin` package was considered and is explicitly
rejected: it only works on Linux, is fragile across Go compiler/toolchain
versions (a plugin must be built with the exact same toolchain version as
the host binary), and is fundamentally incompatible with shipping a single
static cross-compiled binary — there would be nothing for a plugin `.so`
to attach to on Windows or macOS, and ARM64/AMD64 plugin builds would need
to be distributed and matched separately from the host binary.

## Decision

All extensibility in Receiptd is compile-time: a new Element type,
Template, or Provider is added as Go code in this repository (or, in
principle, a fork/vendor of it), self-registers via that package's
`init()` function, and is wired in via a blank import
(`import _ "github.com/harveysandiego/receiptd/templates/weather"`) added at
the composition root (`cmd/receiptd`). This is the same pattern the Go
standard library itself uses for extensibility without a plugin system —
`image.RegisterFormat`, `database/sql.Register`, `net/http/pprof`'s
side-effecting import — applied consistently across every extension point
in this project rather than inventing a bespoke mechanism per point:

- **New Element type**: one file in `receipt/`, registers a JSON factory.
- **New Template**: new package under `templates/`, registers a
  `Template`, selected by name via the registry (`templates.Get`).
- **New Provider**: new package under `providers/<domain>/<name>`,
  implementing that domain's own interface, selected via a `driver:` key
  in YAML config (e.g. `providers.weather.driver: openweather`).
- **New printer transport**: implements `printer.Printer`, wired at
  `cmd/receiptd`'s composition step alongside existing transports.
- **New output format** (e.g. PDF, ANSI preview): implemented as a new
  function alongside `canvas.EncodePNG`/`escpos.Encode`, extracted behind
  an `Output` interface only once a second format is real (see
  `docs/ARCHITECTURE.md` §8, §11 — this interface is deliberately not
  introduced in v1 because only one output format of each kind exists
  today).

## Consequences

- Extending Receiptd requires a rebuild — there is no "drop a `.so` file
  into a plugins directory" story, and there will not be one.
- The single-static-binary and cross-compilation story stays simple: one
  build step produces one binary per platform/arch, always, regardless of
  how many Elements/Templates/Providers exist.
- Every extension point uses the *same* mechanism, so a contributor who
  learns how to add a new Template already knows the shape of adding a new
  Provider or Element type — there's one pattern to learn, not several.
- Runtime, third-party (out-of-tree) extensibility is not supported. Users
  who want a Provider or Template this project doesn't ship must fork or
  submit it upstream. This is an accepted limitation given the deployment
  goals — see Alternatives below for what was traded away.

## Alternatives considered

- **Go's `plugin` package**: rejected — see Context. Linux-only,
  toolchain-version-fragile, incompatible with the static/cross-compiled
  binary goal.
- **A scripting-language extension layer** (e.g. embedding Lua/Starlark
  for user-defined Templates/Providers): considered as a way to get
  runtime extensibility without native plugins, but rejected for v1 as
  disproportionate complexity (an embedded interpreter, a sandboxing
  story, a second language's worth of API surface to design and document)
  relative to the project's current scope. Not ruled out permanently if a
  strong future need emerges, but explicitly not part of the frozen
  architecture today — would need its own ADR if proposed later.
- **Reflection-based auto-discovery** (e.g. scanning for types implementing
  an interface at startup): rejected — Go doesn't support this without
  explicit registration somewhere anyway (there's no runtime type
  scanning across packages without them being imported first), so it adds
  indirection without removing the blank-import requirement it's trying to
  avoid.
- **Config-driven dynamic dispatch as a second mechanism alongside
  registration** (e.g. a YAML-defined Template built from primitives
  rather than Go code): considered for simple Templates, but rejected as
  a second extension mechanism to maintain conceptually alongside the
  registry pattern — see `CLAUDE.md`'s "How to avoid architectural drift."
  If a real need for user-authored Templates without a rebuild emerges,
  it should be proposed and designed deliberately, not backed into.
