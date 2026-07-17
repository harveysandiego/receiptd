# Versioning Policy

Receiptd follows [Semantic Versioning 2.0.0](https://semver.org/):
`MAJOR.MINOR.PATCH`.

## During 0.x (current)

Per the SemVer spec itself, **anything may change at any minor version
while the major version is `0`** — 0.x is explicitly the "anything can
still change" phase. In practice, Receiptd applies that latitude
narrowly rather than fully using it:

- **Breaking changes are flagged, not silent.** Any change that breaks
  the REST API contract, the Receipt JSON schema, the CLI's flags/output,
  or the config file format is called out explicitly in
  [CHANGELOG.md](CHANGELOG.md) under a `### Changed` or `### Removed`
  heading with a clear description, and carries the `breaking change`
  label on its PR/issue (see [.github/labels.yml](.github/labels.yml)).
  It is not treated as a routine patch bump just because 0.x technically
  allows it.
- **PATCH** (`0.1.x`): bug fixes and internal changes with no
  compatibility impact.
- **MINOR** (`0.x.0`): new features (Element types, endpoints, templates,
  providers) and any breaking change, until 1.0. This mirrors how much of
  the Go ecosystem itself treats 0.x MINOR bumps.

## What counts as the public API surface

For the purposes of this policy, "the public API" means:

- The REST API request/response shapes under `/api/v1/*`
- The Receipt JSON schema (`receipt.Receipt` and its Element types)
- The `receipt` CLI's flags, subcommands, and output format
- The YAML configuration file format
- Exported Go identifiers, **if** Receiptd is ever imported as a library
  by something outside this repo — not expected while everything lives
  under `internal/` (see `docs/ARCHITECTURE.md` §1), but noted for
  completeness

Internal package structure, unexported identifiers, and implementation
details documented in `docs/ARCHITECTURE.md` are not part of this
contract and can change in a patch release, provided the architecture
documentation and any relevant ADR are updated alongside the change (see
[CONTRIBUTING.md](CONTRIBUTING.md)).

## 1.0 and beyond

Reaching `1.0.0` is a statement that the public API surface above is
stable enough to commit to standard SemVer guarantees:

- **MAJOR**: breaking changes to any of the surfaces listed above.
- **MINOR**: backwards-compatible new functionality.
- **PATCH**: backwards-compatible bug fixes.

There's no fixed date for 1.0 — it happens once the REST API and Receipt
schema have been exercised by real templates/providers (Milestone 6) and
have gone through at least one real backwards-compatibility test (see the
`Receipt.Version` compatibility-test recommendation in
`docs/ARCHITECTURE.md` §11), not on a calendar-driven schedule.

## Deprecation

Once 1.0 is reached, a field, endpoint, or flag being removed is
deprecated for at least one MINOR release before removal: documented in
`CHANGELOG.md`, noted in relevant godoc comments, and (where the API
shape allows it) accepted-but-warned in the API/CLI before being dropped
in the next MAJOR. Before 1.0, deprecation notice is best-effort rather
than guaranteed, consistent with the "flagged, not silent" principle
above.
