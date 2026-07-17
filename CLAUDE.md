# CLAUDE.md — Development Guide

This document is the long-term development guide for Receiptd. It is
written for two audiences at once: **human contributors** and **AI coding
assistants** (including future Claude Code sessions) working in this
repository. It is not a prompt and it is not a substitute for
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — read that first for *what*
the system looks like. This document is about *how work gets done* inside
that shape.

If anything here ever conflicts with `docs/ARCHITECTURE.md`, the
architecture document wins for design questions; this document wins for
process/workflow questions. If they seem to disagree, that's a bug in one
of the two documents — raise it rather than picking a side silently.

---

## Project philosophy

Receiptd exists to make a physical thermal printer disappear as a concern
for anything that wants to print to it. Every design choice traces back to
one sentence: **no client, anywhere, should need to know it's talking to
an ESC/POS printer.** Not its width, not its cut command, not its
codepage. That's the test to apply when something doesn't feel right about
a proposed change: does it leak a printer detail somewhere it shouldn't?

The second, equally load-bearing priority, stated explicitly by the
project owner during design: **this codebase is meant to be maintained for
years, not shipped once and abandoned.** Every trade-off in this document
and in `docs/ARCHITECTURE.md` was decided in that direction — sometimes at
the cost of a slightly slower path to a first working version.

## Architecture principles

These are the recurring judgment calls made throughout
`docs/ARCHITECTURE.md`, extracted so they're easy to reapply to new
decisions:

- **Discover interfaces at the second real use, not before.** An interface
  with exactly one implementation and no near-term second one is usually
  a cost with no benefit. `render/layout.Font` is the one deliberate
  exception in this codebase — flagged explicitly as an exception, not
  treated as the default.
- **Packages are grouped by responsibility, not created speculatively.**
  A package should earn its existence by holding code that genuinely
  varies independently from its neighbors (see `docs/ARCHITECTURE.md` §11
  for the ~19→13 package reduction and the reasoning behind each fold).
  When in doubt, put new code in an existing package as a new file; split
  it out later if and when a second implementation or a real dependency
  conflict appears.
- **The dependency graph is a DAG, always.** `docs/ARCHITECTURE.md` §11
  documents the full dependency order. Before adding an import, check it
  doesn't point "backwards" in that order. The one relationship to watch
  most carefully: `app` implements `queue.Processor` structurally, so
  `queue` must never import `app`.
- **Typed errors, not string matching.** Every error meaningful to a
  caller crosses package boundaries as an `apperr.Error` with a `Kind`.
  See "Error handling philosophy" below and
  `docs/adr/0005-error-handling.md`.
- **Capabilities and transport are different concepts and never mix.**
  `printer.Profile` (what the printer can do) and `printer.Connection`
  (how to reach it) are separate types specifically so that `render/*`
  never has a code path that could accidentally depend on how a printer is
  connected. Don't add a function to `render/*` that accepts a
  `Connection`, even "just for logging."
- **Raster-first.** Text, images, QR codes, barcodes — everything ends up
  as pixels on a `Canvas` before it becomes ESC/POS bytes. Real ESC/POS
  commands are limited to init/feed/cut and the raster print command
  itself. Don't reach for a vendor-specific native text/QR/barcode command
  as a shortcut — see `docs/adr/0002-raster-rendering.md`.

## Coding standards

- `gofmt` and `goimports` clean, `golangci-lint run ./...` clean — both
  enforced in CI, not negotiable per-PR.
- Small interfaces (see Architecture principles above). If you're writing
  an interface with one method, that's usually right; if you're writing
  one with six, ask whether it's really one concept.
- Structural typing is used deliberately throughout (no `var _
  SomeInterface = (*Impl)(nil)` declarations needed as documentation —
  Go's compiler already enforces this at any call site that needs it).
- Concrete types by default; interfaces only where the architecture calls
  for one. `queue.Queue` is a concrete struct on purpose — see
  `docs/ARCHITECTURE.md` §2.
- Registry + blank-import is this project's one extension mechanism
  (Elements, Templates, Providers). Don't introduce a second mechanism
  (plugins, reflection-based discovery, config-driven dynamic dispatch)
  alongside it — see `docs/adr/0004-extension-model.md`.
- Context (`context.Context`) is the first parameter on anything that does
  I/O (printer sends, asset store access, provider HTTP calls, queue
  processing) — always, not just where convenient.

## Package responsibilities

The canonical list lives in `docs/ARCHITECTURE.md` §1 — this is a quick
reference, not a replacement:

| Package | Responsibility |
|---|---|
| `apperr` | Typed error taxonomy. Zero internal dependencies. |
| `receipt` | Element interface + concrete types, Receipt struct, JSON polymorphism. |
| `printer` | Profile, Connection, Printer interface, network transport. |
| `render/layout` | Receipt + Profile → Document. Owns the Font interface. |
| `render/canvas` | Document → Canvas (bitmap) + PNG encoding. |
| `render/escpos` | Canvas → ESC/POS bytes. |
| `queue` | Job, Store (bbolt/memory), Queue, retry/backoff. |
| `templates` | Template interface + registry. |
| `assets` | Named asset storage (Get/Put/Delete/List). |
| `config` | YAML config struct, loader, validation. |
| `auth` | Bearer/basic auth middleware. |
| `app` | Service layer wiring everything together; the only thing `api`/`webui` call into for business logic. |
| `api` | REST handlers. |
| `webui` | HTML handlers. |
| `cmd/receiptd` | Composition root — the only place a `Connection` is constructed. |
| `cmd/receipt` | CLI. |

Before adding a new package, re-read `docs/ARCHITECTURE.md` §11's
"unnecessary packages" reasoning. The default answer to "should this be
its own package" is no, until something concrete forces it (a second
implementation, a real import-graph conflict, a genuinely independent
release cadence).

## Testing philosophy

Tests exist to catch real regressions, not to hit a coverage number. The
per-package testing approach is specified in `docs/ARCHITECTURE.md` §9 —
follow it rather than inventing a new style per package:

- `receipt`: JSON round-trip + `Validate()` behavior per Element type.
- `render/layout`: assertions on the `Document` data structure, not pixels.
- `render/canvas`, `render/escpos`: golden tests (`testdata/`, `-update`).
- `queue`: fake `Store`/`Processor`, scripted `apperr.Kind` failures,
  `-race`.
- `printer`: local `net.Listen` fake server — never real hardware in CI.
- `api`: `httptest` against a fake `app.Service`.
- Hardware: `-tags hardware`, manual checklist, never in default CI.

Assert on `apperr.Is(err, apperr.KindX)`, never on `err.Error()` string
content — messages are free to change wording without breaking tests.

`go test -race ./...` must pass, always. A flaky test is a bug in the test
(or in the code it tests), not something to retry past.

## Definition of Done

Restated from `CONTRIBUTING.md` for AI-assisted work specifically: a
change is not done when the code compiles and looks right. It's done when:

1. Tests exist that would fail if the change were reverted or subtly
   broken, and they pass, including `-race`.
2. `golangci-lint run ./...` and `gofmt`/`goimports` are clean.
3. `docs/ARCHITECTURE.md` and, if relevant, a `docs/adr/` entry are updated
   in the *same* change if any design decision moved.
4. A human has reviewed it. See "AI-assisted development" below — this
   step is not optional regardless of who/what wrote the code.

## Preferred workflow

1. Read the relevant section of `docs/ARCHITECTURE.md` (and any ADR it
   references) before writing code — don't rediscover a decision that's
   already been made and justified there.
2. Write the test first where practical (see "How to implement new
   features" below) — this project is committed to TDD for the milestone
   implementation work in particular.
3. Small, reviewable commits and PRs over one large drop. Each milestone
   in the roadmap is itself meant to be broken into several PRs, not
   landed as one.
4. Run `make ci` locally before pushing — it mirrors what CI actually
   checks, so surprises show up on your machine first.

## How to implement new features

For a **new Element type** (e.g. a hypothetical `image-grid`):

1. Add one new file in `receipt/` defining the struct and its
   `Validate() error`, self-registering via `init()`.
2. Add handling in `render/layout.Build`'s type switch to turn it into
   `Block`(s).
3. Add handling in `render/canvas.Paint` if it needs new drawing logic.
4. Golden test in `render/canvas`, round-trip + validation test in
   `receipt`.
5. Document it in the Element table in `docs/ARCHITECTURE.md` §3.

For a **new Template**: new package under `templates/`, registers via
`init()`, one blank import in `cmd/receiptd`. It should only ever call
`Provider.Current`-style methods and build a `receipt.Receipt` — it must
never call `layout`, `canvas`, `escpos`, or `printer` directly (see
`docs/ARCHITECTURE.md` §4's invariant: only `layout` touches the outside
world downstream of the Receipt being built).

For a **new Provider domain**: new package under `providers/<domain>`,
defining that domain's own interface shape — there is deliberately no
shared cross-domain `Provider` interface to conform to (see
`docs/ARCHITECTURE.md` §1).

For a **new printer transport** (USB/Bluetooth/Serial): implement
`printer.Printer`. This is also the flagged point at which splitting
`printer` into subpackages becomes worth reconsidering (§1) — don't
preemptively split it before a second transport actually exists.

## How to avoid architectural drift

- If you find yourself wanting to add a parameter of type
  `printer.Connection` to any function in `render/*`, stop — that's the
  one invariant this design leans on hardest. Solve the problem a
  different way, or raise it as an architectural question first (see
  `CONTRIBUTING.md`).
- If you find yourself wanting a second, parallel way to register
  something (config-driven instead of blank-import, for instance), that's
  a second extension mechanism — raise it rather than adding it quietly.
- If a package you're editing starts needing something "just this once"
  from a package below it in the dependency order, that's exactly how
  accidental cycles start. Check `docs/ARCHITECTURE.md` §11's dependency
  graph before adding the import.
- Any change to a documented interface, package boundary, or the Receipt
  model itself needs `docs/ARCHITECTURE.md` updated in the same PR — see
  "How architectural changes should be proposed" in `CONTRIBUTING.md`. A
  code change that quietly contradicts the document is worse than one
  that's merely undocumented, because it actively misleads the next
  reader (human or AI) who trusts the document.

## Refactoring philosophy

Refactor when it removes real duplication or fixes a real awkwardness you
hit while implementing something — not speculatively, and not as a
drive-by bundled into an unrelated feature PR. A bug fix doesn't need
surrounding cleanup. If a refactor is worth doing, it's worth its own PR
with its own description of what got simpler and why, so it can be
reviewed on those terms rather than hidden inside a feature diff.

## Performance philosophy

Don't optimize for a guess. `docs/ARCHITECTURE.md` §11 flags two specific
places this applies on day one: `escpos.Encode`'s image-chunking logic
should ship as a no-op until real hardware testing shows the TM-m30II
actually needs it, and `printer.Transport` dispatch should stay a single
`case "network"` until a second transport is real. The general rule this
generalizes to: build the general mechanism when the second concrete case
shows up, not in anticipation of it. Where performance work is genuinely
warranted (e.g. `render/canvas.Paint` on a large receipt), profile first
and let a benchmark justify the change, rather than reasoning about it in
the abstract.

## Error handling philosophy

See `docs/ARCHITECTURE.md` §5 and `docs/adr/0005-error-handling.md` for
the full design. In practice:

- Wrap with `apperr.Wrap(kind, op, err)` at the point an error is first
  known to have a meaningful `Kind` — a dial failure in `printer` is
  `KindTransient` right there; a missing asset in `assets` is
  `KindNotFound` right there. Don't defer classification to a caller who
  has less context than you do at the point of failure.
- Never introduce a second error convention (sentinel errors compared with
  `==`, ad hoc string-prefixed errors, a second wrapping type) alongside
  `apperr` — one taxonomy, used everywhere a caller needs to make a
  decision based on *what kind* of failure occurred.
- `Validate()` methods are fast and local (no I/O) by design; I/O-backed
  existence checks (does this asset actually exist) belong in the stage
  that already does I/O (`layout.Build`), not in `Validate()`. Don't blur
  this line to save a function call.

## Documentation expectations

- Every exported identifier gets a godoc comment that earns its keep —
  explain *why* something works the way it does when that's non-obvious,
  not a restatement of the name.
- `docs/ARCHITECTURE.md` is the living design reference; it changes only
  alongside the code it describes, in the same PR.
- `docs/adr/` records *why* a major decision was made, once, so it's never
  re-litigated from scratch — new ADRs are added for new major decisions,
  existing ones are not rewritten after the fact (mark one "Superseded by
  000X" instead of editing history away).
- The README stays accurate for a first-time visitor — if a milestone
  changes what's actually usable today, update the "Current status" and
  roadmap checkboxes in the same PR that lands it.

## AI-assisted development

Receiptd's architecture was designed collaboratively with Claude Code, and
implementation is being carried out with AI assistance milestone by
milestone. This is a deliberate, disclosed choice, not an incidental
detail — and the project welcomes AI-assisted contributions from anyone,
under the same terms as any other contribution:

- **All AI-generated code must be reviewed by a human before merge.** No
  exception for "the AI wrote the tests too, so it must be right" — tests
  written by the same process that wrote the code under test can share the
  same blind spot, so review both.
- **Tests and linting must always pass** — this is checked by CI
  regardless of how the code was produced, and is not relaxed for
  AI-assisted PRs.
- **The architecture is not something an AI assistant should change
  casually.** An assistant working in this repository should treat
  `docs/ARCHITECTURE.md` as a design a human already thought hard about
  and agreed to freeze — propose a change through the process in
  `CONTRIBUTING.md` rather than "improving" a package boundary or
  interface mid-task because it seems cleaner in isolation.
- **Disclosure over attribution.** This project discloses that it's built
  with AI assistance at the project level — this file, the README
  acknowledgements, and PR descriptions where relevant — rather than via
  per-commit `Co-Authored-By` trailers. A trailer on every commit implies
  a form of individual authorship credit that doesn't fit how an AI
  assistant is actually used here (a tool under a human's direction and
  review, not an independent contributor), and clutters `git log`/`git
  blame` with a credit that's true of nearly every commit and therefore
  says nothing about any specific one. If a specific PR is substantially
  AI-generated and that fact is relevant context for reviewers (e.g. it
  changes how carefully to scrutinize it), say so once, in the PR
  description — that's the right level of granularity, not a
  per-commit trailer.

If you are an AI assistant reading this file to orient yourself in the
repository: the two documents to read before making any non-trivial change
are this one and `docs/ARCHITECTURE.md`. If a task seems to require
contradicting either, stop and surface that explicitly rather than
resolving the tension silently.
