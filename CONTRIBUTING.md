# Contributing to Receiptd

Thanks for considering a contribution. This document explains how the
project is developed day to day. If you only read one other document
first, read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — it explains the
design this project is built around and the reasoning behind it, so you're
not left guessing why a package or interface looks the way it does.

## Development setup

Requirements:

- Go (current stable or previous stable release — see the CI matrix in
  `.github/workflows/ci.yml`)
- `golangci-lint` (`make install-tools`)
- Optionally: [pre-commit](https://pre-commit.com/) (`pip install pre-commit`
  or `brew install pre-commit`)

```sh
# Update this URL if you're working from a fork.
git clone https://github.com/harveysandiego/receiptd.git
cd receiptd
make install-tools
pre-commit install   # optional but recommended
make build
make ci              # fmt, vet, lint, test -race, coverage — mirrors CI
```

If you use VS Code with Dev Containers (or GitHub Codespaces), opening
the repo picks up `.devcontainer/devcontainer.json` automatically — Go,
`golangci-lint`, and the recommended extensions are preinstalled, and
`CGO_ENABLED=0` is set to match how Receiptd is actually built/released.
This is optional; the plain setup above works anywhere.

You do not need a physical printer to develop most of Receiptd. The
`render/*` and `queue` packages are testable entirely offline; `printer`
tests use a local `net.Listen` fake server. Hardware-dependent tests are
gated behind a `hardware` build tag and are not part of default CI (see
`docs/ARCHITECTURE.md` §9).

## Coding standards

- Run `gofmt` / `goimports` — enforced in CI, not a style debate.
- `golangci-lint run ./...` must pass; see `.golangci.yml` for the enabled
  linters and why.
- Keep interfaces small and only introduce one when a second real
  implementation exists (or, rarely, when explicitly justified in
  `docs/ARCHITECTURE.md`, as with `render/layout.Font`). See "How to avoid
  architectural drift" in `CLAUDE.md`.
- Errors that cross a package boundary and are meaningful to a caller
  should be wrapped with `apperr.Wrap(kind, op, err)` — see
  `docs/ARCHITECTURE.md` §5 and `docs/adr/0005-error-handling.md`. Don't
  invent a second error convention alongside it.
- Prefer table-driven tests. Prefer `errors.Is`/`apperr.Is` assertions over
  string-matching on error messages.
- Comments explain *why*, not *what* — see the "Documentation
  expectations" section of `CLAUDE.md`.

## Branch strategy

- `main` is always releasable — CI must pass on every commit on `main`.
- Work happens on short-lived feature branches off `main`, named
  descriptively (`render-layout-columns`, `fix-queue-retry-backoff`). There
  is no long-lived `develop` branch.
- Rebase your branch on `main` before opening a PR if it's fallen behind;
  avoid merge commits from `main` into your branch.

## Commit message conventions

This project loosely follows [Conventional Commits](https://www.conventionalcommits.org/),
which also drives the grouped changelog in release notes
(`.goreleaser.yml`):

```
<type>(<optional scope>): <short summary>

<optional body — the "why", not a restatement of the diff>
```

Common types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`.
Examples:

```
feat(render/layout): wrap long text elements at word boundaries
fix(queue): only retry apperr.KindTransient failures
docs(adr): add 0005-error-handling
```

Keep commits small and reviewable — one logical change per commit. Squash
fixup commits before requesting review where practical.

## Testing requirements

- New code needs tests. The bar is "would a bug here be caught by a test,"
  not 100% coverage for its own sake.
- `render/canvas` and `render/escpos` changes need golden test coverage
  (`testdata/`, `-update` flag to regenerate) — see `docs/ARCHITECTURE.md`
  §9 for the full per-package testing table.
- Before pushing: `go test ./...`, `go test -race ./...`, and
  `golangci-lint run ./...` must all pass locally (`make ci` runs all of
  these plus formatting/vet checks in one command).
- Don't gate CI-required tests behind build tags to make them pass; if a
  test needs real hardware, it belongs behind `-tags hardware` and is
  excluded from CI by design, not by accident.

## Benchmarking

`make bench` runs `go test -bench=. -benchmem -run='^$' ./...` — it's a
no-op today because no benchmarks exist yet, and it's intentionally not
part of `make ci`: benchmark numbers are noisy in shared CI runners and
are meant to be run and compared locally, not gated on.

Write a benchmark (`func BenchmarkX(b *testing.B)`, alongside the code it
measures) when:

- You're changing something on a genuinely hot path — `render/canvas.Paint`
  (painting a full receipt), `render/escpos.Encode`, or `render/layout.Build`
  for large/complex Receipts — and want to show a change actually helps
  rather than just seeming like it should.
- A PR is specifically about performance. In that case, include
  before/after benchmark output in the PR description.

Per the "Performance philosophy" in [CLAUDE.md](CLAUDE.md): don't add a
benchmark (or an optimization) speculatively for code that isn't known to
be a bottleneck — profile or benchmark first, let the numbers justify the
change.

## Fuzz testing

No fuzz tests exist yet — there's no parser to fuzz until Milestone 1+
lands. Once the relevant code exists, Go's built-in fuzzing
(`func FuzzX(f *testing.F)`, `go test -fuzz=FuzzX`) is encouraged,
particularly for anything that parses untrusted input:

- **`receipt`'s JSON decoding** — a Receipt document may come from an
  untrusted API caller; the registry-based `Element` unmarshaling in
  particular is a good fuzz target once it exists.
- **`render/escpos.Encode`** — encoding is deterministic and byte-exact by
  design (golden-byte tested, per `docs/ARCHITECTURE.md` §9), which also
  makes it a good fuzz target for panics/invalid output on malformed
  `Canvas` input.
- **`render/layout.Build`** — arbitrary/adversarial Receipts (deeply
  nested `columns`, pathological `table` dimensions) should fail
  gracefully with an `apperr`, never panic.
- **Any future parser** — Markdown-to-Receipt conversion, if/when it's
  built, is exactly the kind of untrusted-text-to-structured-data path
  fuzzing is good at.

A fuzz test isn't required to merge the initial implementation of any of
the above, but a contributor adding one is welcome, and a maintainer may
ask for one specifically on a parsing-heavy PR. Corpus files (`testdata/fuzz/`)
generated by `go test -fuzz` are committed like any other test fixture
once a fuzz test finds and fixes a real crasher.

## Definition of Done

A change is done when:

- [ ] It does what the issue/PR describes, with tests proving it
- [ ] `go test -race ./...` and `golangci-lint run ./...` pass
- [ ] Documentation is updated alongside the code — godoc comments on
      exported identifiers, README if user-facing behavior changed,
      `docs/ARCHITECTURE.md` and/or an ADR if a design decision changed
- [ ] `CHANGELOG.md`'s `[Unreleased]` section has an entry for any
      user-facing change (new endpoint, CLI flag, Element type, config
      option, bug fix) — see [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
      Purely internal changes (refactors, test-only changes, CI/tooling)
      don't need an entry.
- [ ] No unrelated formatting churn or drive-by refactors bundled in
- [ ] The PR template's checklist is filled in honestly, not rubber-stamped

## Review expectations

- All changes land via pull request, including the maintainer's own —
  no direct pushes to `main`.
- Reviews focus on correctness, test coverage, and consistency with
  `docs/ARCHITECTURE.md`; nitpicks on style are handled by the linters, not
  by review comments.
- Small, focused PRs get reviewed faster than large ones. If a change is
  naturally large (e.g. landing a whole milestone), consider splitting it
  into a stack of smaller PRs where the architecture allows it.

## How architectural changes should be proposed

`docs/ARCHITECTURE.md` is frozen: it should not be changed casually, and a
PR should not silently drift from it (different interface shape, extra
package, different error handling) without updating the document in the
same PR.

If you believe something in the architecture is genuinely wrong or
inadequate — not just a matter of taste — the process is:

1. Open an issue describing the problem with the *current* design, using
   concrete evidence if possible (a real limitation you hit implementing
   something, not a hypothetical). Reference the specific section of
   `docs/ARCHITECTURE.md` or ADR being questioned.
2. Discuss alternatives in the issue before writing code. Architectural
   changes are evaluated on the same terms the original decisions were:
   does this reduce or add unnecessary abstraction, does it keep the
   dependency graph acyclic, does it serve a real present need rather than
   a speculative future one.
3. Once there's agreement, the PR implementing the change must also update
   `docs/ARCHITECTURE.md` and add or amend an ADR under `docs/adr/`
   explaining the context, decision, consequences, and alternatives
   considered — the same way the existing ADRs do. A code change that
   contradicts the documented architecture without updating it will be
   asked to do one or the other before merge.

Small clarifications that don't change behavior (typos, better wording,
filling a documented gap) don't need this process — just send a docs PR.

## AI-assisted contributions

Receiptd was designed and is being built with AI assistance (Claude Code).
AI-assisted contributions from others are welcome under the same bar as
any other contribution — see the "AI-assisted development" section of
[CLAUDE.md](CLAUDE.md) for the project's full position on this, including
the review and testing expectations that apply regardless of how the code
was written.

In short: no `Co-Authored-By` trailer needed on your commits for this —
see CLAUDE.md for why. A one-line note in the PR description is enough if
you think it's useful context for the reviewer.
