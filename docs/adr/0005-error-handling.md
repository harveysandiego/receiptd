# 0005. Typed errors via a Kind + Op + wrapped-cause taxonomy

Status: Accepted

## Context

The same error needs to be interpreted correctly by several different
consumers, each asking a different question:

- The **REST API** needs to know what HTTP status code to return.
- The **queue worker** needs to know whether a failure is worth retrying
  or should fail the job immediately.
- **Logs** need structured detail (what kind of failure, in what
  operation) rather than a free-text message to grep.
- **Tests** need to assert on failure *category* without being coupled to
  exact wording, which is free to change.

A plain `error` (or Go's `errors.New`/`fmt.Errorf`-only style) doesn't
carry enough structure to answer any of these questions without string
matching on the error message — brittle, and exactly the kind of thing
that breaks quietly when someone rewords a message.

## Decision

A small package, `apperr`, defines a closed set of error kinds and a
wrapping type carrying that kind plus the operation that produced it:

```go
type Kind int

const (
    KindUnknown Kind = iota
    KindValidation     // bad Receipt / bad request — 400, no retry
    KindNotFound        // asset/template/job not found — 404, no retry
    KindTransient       // printer offline, transport timeout — retry
    KindPermanent        // renderer failure, unrecoverable — 500, no retry
    KindUnauthorized      // missing/bad token — 401, no retry
)

type Error struct {
    Kind Kind
    Op   string // e.g. "layout.Build", "assets.Get"
    Err  error
}

func Wrap(kind Kind, op string, err error) *Error
func Is(err error, kind Kind) bool // errors.As + Kind match
```

This is deliberately the same shape as Rob Pike's Upspin `errors` package
— a well-precedented answer to "typed errors that need to map cleanly
across layers," not a novel invention. Every package that returns a
domain-meaningful error wraps it with `apperr.Wrap(kind, op, err)` at the
point the kind is first known — e.g. a dial failure in the `printer`
network transport is wrapped `KindTransient` right there, a missing named
asset in `assets.Store.Get` is wrapped `KindNotFound` right there. Callers
consume the `Kind`, never the error string.

Per-consumer mapping:

- **API**: a small `Kind → http.StatusCode` table (`KindValidation→400,
  KindNotFound→404, KindUnauthorized→401, KindTransient→503,
  KindPermanent→500`).
- **Queue retry**: `apperr.Is(err, apperr.KindTransient)` is the *only*
  condition that consumes retry budget — every other kind fails the job
  immediately.
- **Logs**: structured fields (`logger.With("kind", err.Kind, "op",
  err.Op)`) via `charmbracelet/log`, not string interpolation.
- **Tests**: `apperr.Is(err, apperr.KindNotFound)`-style assertions.

`Validate()` methods across the codebase (e.g. `receipt.Element.Validate`)
are fast and local by design and return validation failures wrapped as
`KindValidation`; I/O-backed existence checks are deliberately deferred to
the stage that already performs I/O (see `docs/adr/0001-receipt-model.md`
for the Asset-specific example of this split).

## Consequences

- Every layer that needs to make a decision based on failure type
  (HTTP status, retry-or-not, log severity, test assertion) reads the same
  `Kind` field — one taxonomy, one source of truth, instead of each
  consumer inventing its own classification of the same underlying errors.
- Adding a new failure mode means picking (or, rarely, adding) a `Kind`,
  not inventing a new error-handling convention — keeps error handling
  consistent as the codebase grows.
- The `Kind` enum is intentionally small and closed — resisting the urge
  to add a highly specific kind for every new error site is important;
  most new failures should map onto one of the existing five kinds by
  asking "what should the API/queue/logs *do* about this," not "how do I
  describe this precisely."
- `errors.Join` (stdlib, Go 1.20+) is used to aggregate multiple
  validation failures (e.g. every failing Element in a Receipt) before a
  single outer `apperr.Wrap(KindValidation, ...)` — no third-party
  multi-error package is needed.

## Alternatives considered

- **Sentinel errors compared with `==`** (`var ErrNotFound = errors.New(...)`):
  rejected as the primary mechanism — doesn't carry the operation context
  (`Op`) or compose with wrapping as cleanly, and encourages a growing list
  of ad hoc sentinels with no shared structure for the API/queue mapping
  layers to key off.
- **String-prefixed or string-matched errors** (e.g. checking
  `strings.HasPrefix(err.Error(), "not found")`): rejected — brittle by
  construction; a wording change silently breaks every consumer relying on
  it, including tests.
- **A third-party multi-error/wrapping library** (e.g. `hashicorp/go-multierror`,
  `pkg/errors`): rejected — `errors.Join` and `errors.Is`/`As` in the
  standard library (Go 1.20+) already cover what this project needs;
  adding a dependency for functionality the stdlib now provides would be
  unnecessary complexity.
- **A separate error type per package** (each package defining its own
  local error struct): rejected — this is close to what motivated `apperr`
  in the first place; a shared taxonomy is what lets the API, queue, and
  logging layers each write one small piece of mapping code instead of one
  per producing package.
