# 0030. Build version surfaced at every operator-facing seam

Status: Accepted

## Context

`version`, `commit`, and `date` are injected at build time via `-ldflags`
(`.goreleaser.yml`, `Dockerfile`) into both binaries' `main` packages. Until
now they surfaced in exactly two places: the daemon's startup banner, which
scrolls out of a terminal or rotates out of a log, and the `receipt` CLI's
Cobra `--version` flag.

That left three gaps, each hit by a different operator:

- `.github/ISSUE_TEMPLATE/bug_report.md` asks a reporter to run
  `receiptd --version`. That flag did not exist — `main()` registered only
  `-config`, so the documented command exited with a flag error. The
  project was asking for a version by a means that could not produce one.
- `web.enabled` defaults to `false`, so a default deployment exposed the
  running version on no HTTP surface at all. Nothing could ask a running
  daemon what it was.
- An operator working in the Web UI — plausibly the only interface they
  have, running the published container — could not see the version without
  shelling into it.

The container image was already covered: the release workflow passes
`VERSION`/`COMMIT`/`DATE` as build args and applies OCI labels, so
`docker inspect` has always answered this.

## Decision

Surface build identity at all four operator-facing seams: a `--version`
flag on `receiptd`, `GET /api/v1/version`, a footer on every Web UI page,
and a `receipt version` subcommand reporting the CLI's own version
alongside the daemon's.

Four decisions shaped how, rather than what:

**Build identity rides on `app.Service` for `webui`.** `internal/webui`
reads everything through the `app.Service` seam (`docs/ARCHITECTURE.md`
§10) and must not reach into `main`. `Service.Build` is display-only data
copied down by `cmd/receiptd` at startup — the precedent
`ConnectionSummary` already set for exactly this shape of problem.

**`internal/api` takes the values directly instead.** That package
deliberately depends on no `app` types, declaring a narrow structural
interface per handler. Build identity is a fixed process-lifetime value,
not something to ask a service for per request, so `NewVersionHandler`
takes the three strings and `internal/api` keeps its clean import list.

**The Web UI footer uses an envelope, not a per-page field.** `render`
wraps a page's own model in a `pageEnvelope{Page, Version}` and `base.tmpl`
passes `.Page` into every block. The version is sitewide chrome, not page
data; putting a `Version` field on all six page models would contradict
§10's "page-owned presentation model" rule, which exists precisely to stop
fields a page doesn't own from appearing on it.

**The endpoint sits inside the Bearer-protected mux.** It is registered on
the same `apiMux` as every other route, so it inherits `auth.Bearer` when
auth is enabled, with no special-casing.

## Consequences

A version is now readable from wherever an operator already is, and the bug
report template describes a command that exists.

`render` and `renderError` each grew a `version` parameter, threaded through
29 call sites in `internal/webui`. That is real noise at every call site,
accepted because the alternatives were worse: the envelope keeps page models
honest and keeps `render` ignorant of `app`, which its doc comment claims and
which makes it testable without a `Service`.

`app.Service` now carries a field that is not application state. That is a
small dilution of what the service layer means, bounded by the existing
`ConnectionSummary` precedent and by `BuildInfo` having no methods — how to
display it stays each caller's decision.

The version is disclosed to anyone who can reach the Web UI or the API with
a credential. That is not new information for an authenticated operator, but
it does mean an unauthenticated deployment (`auth.enabled: false`) now
advertises its exact version to anyone who can reach the port. Operators
running without auth already have a larger problem
(`docs/adr/0021-transport-security-via-reverse-proxy.md`).

## Alternatives considered

**A `Version` field on each page model.** Rejected: six structs and every
construction site would carry a field none of those pages owns, which is the
exact drift §10's page-owned rule was written to prevent, and any new page
could silently forget it.

**A package-level `version` variable in `internal/webui`, set by
`NewRouter`.** Zero call-site churn, but it is mutable global state written
after init and read by every handler — a data race the moment two tests call
`NewRouter` in parallel, in a project committed to `-race` in CI.

**Reading `runtime/debug.ReadBuildInfo()` inside `webui`/`api`.** Needs no
plumbing at all, but it reports the module's version, not the `-ldflags`
values the release process actually stamps, and it would let packages below
the composition root discover their own build environment — the opposite of
`cmd/receiptd` being the only place that knows how this process was
assembled.

**Serving the version endpoint unauthenticated,** so monitoring can poll it
without credentials. Rejected: it would tell anyone who can reach the port
exactly which version to target, and no current consumer needs it. Nothing
stops this being revisited if a real health-check consumer appears — a
health endpoint is a different question from a version endpoint.
