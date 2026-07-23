# 0020. Idempotent print requests via a client-supplied key, deduped at enqueue time

Status: Accepted

## Context

`POST /api/v1/print` (`internal/api/print.go`) is a physical side-effecting
API, not a CRUD endpoint. `app.Service.Print` (`internal/app/service.go`)
validates the posted `receipt.Receipt`, builds a `queue.Job`, and calls
`Queue.Enqueue`, which unconditionally assigns a fresh `Job.ID`
(`queue.Queue.Enqueue`, `internal/queue/queue.go`) and persists it. There is
today no client-supplied key of any kind on the request, on `Job`
(`internal/queue/job.go`: `ID, PrinterName, Receipt, State, Attempts,
LastError, CreatedAt, UpdatedAt`), or on `queue.Store`
(`internal/queue/store.go`: `Save`/`Get`/`List`/`NextPending`). A duplicate
HTTP POST — a client timing out and retrying, a network blip, a
double-click in some future UI, an HTTP client library that retries on a
5xx — is therefore indistinguishable from a second, independent print
request. It produces a second `Job`, and eventually a second physical
receipt is cut and ejected from the printer.

This is the CRUD-vs-physical-side-effect distinction that makes this
harder than the textbook case. For a CRUD resource, "idempotent" usually
means "retrying returns the same response, and no second row gets
written" — a purely informational guarantee. Here, the thing that must
not happen twice is a piece of paper being fed and cut. Getting the
detection wrong in either direction is a real failure, not a cosmetic one:

- **Too weak** (no dedup, today's behavior): a network blip between a
  client and Receiptd during a legitimate retry causes a customer to walk
  away with two copies of the same receipt.
- **Too aggressive** (dedup on Receipt *content* rather than client
  intent): two different customers who each order exactly one identical
  item generate two byte-identical Receipts. If Receiptd treated
  "identical content" as "the same request," the second customer's
  receipt would silently never print. Content equality is not evidence of
  duplicate intent — only the client, who knows whether it is retrying the
  same logical operation or issuing a new one, can supply that signal.

Because `POST /api/v1/print` already only returns a `Job` ID before the
printer is touched (`docs/adr/0003-print-queue.md`), the HTTP response the
client is retrying against was never "the print succeeded" — it was
always "the print was accepted for later, asynchronous processing." Any
dedup decision has to compose with that: it must be made at the point the
`Job` is created (synchronously, inside the HTTP request that calls
`Service.Print`), because the background queue worker
(`queue.Queue`/`Processor`) never sees an HTTP request or its headers —
by the time a `Job` reaches `Process`, whatever key the client sent is
either already recorded on the `Job` or is gone forever.

This ADR does not design multi-worker concurrency (settled by
[0016](0016-queue-concurrency-per-printer-workers.md)), crash recovery
(settled by [0017](0017-queue-lifecycle-crash-recovery.md)), graceful
shutdown ([0018](0018-graceful-shutdown.md)), general job-lifecycle
pruning ([0017](0017-queue-lifecycle-crash-recovery.md)'s territory), or
the queue's own automatic transient-failure retry/backoff mechanics
([0019](0019-retry-pipeline-granularity.md)). Where this decision's
correctness depends on one of those, it says so explicitly and reuses
what that ADR already established, rather than re-deriving it.

## Decision

Receiptd supports idempotency for v1, at exactly one layer: **queue-level
job dedup, decided synchronously at enqueue time**, driven by a client-
supplied key. This is deliberately not "HTTP-level response replay" (a
generic cache of raw HTTP responses keyed by request signature,
independent of the `Job` model) — see Alternatives. The guarantee this
ADR provides is precise: *if a client supplies the same key twice for what
it asserts is the same logical print request, at most one `Job` is ever
active for that key, and therefore at most one physical print is ever
attempted from it.* It says nothing about requests that never supply a
key — those get exactly today's behavior, unchanged.

### Client-supplied key: optional, not required

`POST /api/v1/print` (and the template convenience endpoints that end at
the same `Service.Print` call — `docs/ARCHITECTURE.md` §4 step 6 —
`/api/v1/templates/<name>/print`) accepts an optional `Idempotency-Key`
request header, the now-common convention this project is not inventing
from scratch (popularized by Stripe's API).

Receiptd does **not** reject a request that omits it. An empty/absent key
means "always new, never deduped" — the exact behavior every existing
client already gets today. This follows the same "zero/omitted value means
the pre-existing default behavior" convention already used throughout this
schema (`Text.Size: 0` means unscaled, `Divider.Size: 0` means the base
thickness, `printer.Profile.MaxImageHeightDots: 0` means no chunking).
Requiring the header would be a breaking change on rollout day for every
client that predates this feature, including the CLI and every template
convenience endpoint — none of which have a natural per-request key to
supply without their own changes. Making it optional keeps the wire
contract purely additive: a new optional header, nothing removed or
tightened.

### Persistence requirement: a job record carries an optional key, and can be looked up by it

Each Job must be able to carry an optional, client-supplied idempotency
key alongside its existing fields. A Job with no key (the common case
today) is never a candidate for matching against anything — this is the
same "zero value means absent, not a valid key to match on" rule the rest
of this schema already follows.

The store must additionally provide a way to look up a Job by its
idempotency key, mirroring the existing lookup-by-ID capability: given a
key, return the Job recorded under it, or a clear "no such key" outcome if
none exists. This is a new capability on the store's contract — a real,
if narrow, addition — not a change to any existing lookup, save, or
listing behavior. Whether it's implemented as a full scan (following the
same precedent the existing "find the next pending Job" lookup already
uses) or a secondary index is an implementation choice; for this
project's realistic scale (a Raspberry Pi's print queue is dozens-to-
low-hundreds of Jobs deep, not a scale where a linear scan is a measured
problem), a full scan is expected to be sufficient for v1, with a
secondary index as the natural, contained follow-up if a profile ever
shows otherwise. This cost compounds with
[0017](0017-queue-lifecycle-crash-recovery.md)'s own unbounded full-history
scan at every startup — both are flagged together there so a future
pruning/retention ADR can address both at once.

Expiry is **not** the store's concern: the lookup only answers "does a
record exist under this key," not "does it still count." Whether a found
key is still within its validity window is decided by the caller (see
"Lifetime and expiry" below) — the same separation of concerns
`printer.Profile`/`Connection` already models for a different pair of
concepts.

Carrying an idempotency key is a backward-compatible additive change to
what a Job persists — an existing record with no key simply has none, no
migration needed. The lookup capability is, honestly, a real addition to
the store's contract, but a narrow and self-contained one: every existing
lookup, save, and listing behavior is untouched, and both of this
project's storage backends live in this repository and are updated in the
same change — there is no external implementation this could break
silently. This is the same kind of narrow, justified store extension
[0016](0016-queue-concurrency-per-printer-workers.md) already established
for atomic claiming.

### Making the check-then-act at enqueue time atomic

Looking a key up, deciding what to do (create, return existing, reject, or
reactivate — see below), and then persisting that decision is a
check-then-act sequence: two concurrent requests racing on the same
brand-new key could both miss the lookup and both create a Job, exactly
the kind of race [0016](0016-queue-concurrency-per-printer-workers.md)
already had to close for claiming a pending Job for processing. This ADR
reuses that same invariant rather than inventing a second one: the whole
"look up by key, decide, then persist" sequence for a given key must run
as one atomic operation with respect to any other concurrent attempt on
that key, exactly as
[0016](0016-queue-concurrency-per-printer-workers.md) requires for its own
claim operation. No two concurrent requests can ever both observe "no
existing Job for this key" and both create one. How each storage backend
provides that atomicity is an implementation choice, not part of this
decision.

### What makes two requests "the same"

The key alone is not sufficient to treat a request as a legitimate retry.
If the same `Idempotency-Key` arrives with a *different* Receipt or a
different printer name than the Job already stored under that key, that
is a client-side bug (a key generator reused across two logically
different print requests), not a retry, and is rejected outright —
`apperr.KindValidation` (the existing 400 mapping in `docs/adr/0005-error-
handling.md`'s table), not silently overwritten and not silently printed
twice. Because a Job already carries its full Receipt and printer name,
this comparison needs no new fingerprint/hash field — the incoming
request's Receipt and printer name are compared directly for equality
against the previously stored Job's own fields before deciding a request
is a genuine retry.

### Lifetime and expiry

An idempotency key is valid for **24 hours** from the Job's creation time
— a common, defensible default (Stripe uses the same window) and small
enough that an unbounded-growth concern on a small embedded store never
materializes: expired keys are never deleted, they are simply no longer
matched.

Enforcement is **lazy, at lookup time only** — no active sweep, no
background process, no scheduler. After a lookup by key returns a Job, the
caller checks whether it's older than the retention window; if it is, the
key is treated exactly as if no record existed — the caller falls through
to creating a brand-new Job as if the key had never been used. This is the
simpler of the two options available, and nothing about this project's
stated minimalism argues for a sweep instead: a sweep would need its own
schedule, its own failure mode, and would delete or mutate Job rows whose
retention is properly
[0017](0017-queue-lifecycle-crash-recovery.md)'s decision to make — this
ADR intentionally does not touch whether or when a Job row itself is ever
deleted, only whether its idempotency key still "counts" for matching
purposes. The 24-hour window is a fixed value for v1, not yet exposed as a
configuration option — a knob worth adding once an operator actually needs
a different window, not before.

### Failure-mode decisions at enqueue time

`Service.Print`'s enqueue path, when a non-empty key is supplied, becomes:

1. Look up an existing Job for the key, atomically with the decision below
   (see "Making the check-then-act atomic"), applying the 24-hour
   liveness check above.
2. **No matching, non-expired Job exists** → proceed exactly as today:
   create a new Job (now carrying the supplied key), enqueue it, return
   its new ID.
3. **A matching Job exists, but its Receipt or printer name differ from
   this request** → reject with `apperr.KindValidation`, no Job created or
   touched.
4. **A matching Job exists and matches this request, in `Pending` or
   `Running`** → return that Job's existing ID. This is the core of the
   feature: the client that retried gets back the same Job it already
   created, and nothing is enqueued a second time.
5. **A matching Job exists, `Done`** → return that Job's ID. The print
   already happened; a second attempt must not be queued.
6. **A matching Job exists, `Failed`** → this is the case with a real
   choice to make, and the choice here is: **allow a fresh attempt**,
   rather than handing back the stale failure forever. A `Failed` Job
   never caused a physical print (that is precisely what "failed" means
   in this system, per `docs/adr/0003-print-queue.md`), so re-attempting
   it under the same key carries none of the "might print twice" risk
   this ADR exists to prevent, and handing a client a permanently dead key
   after one transient printer hiccup would make the feature actively
   worse than not having it — normal HTTP-client retry behavior (the exact
   scenario this ADR targets) would then need the client to mint a brand
   new key just to get the print to happen at all, defeating the purpose
   of supplying a stable key in the first place. The chosen behavior: the
   *same* Job (same ID, same key, same Receipt, same creation time) is
   reactivated in place — state set back to `Pending`, `Attempts` **reset
   to zero**, `LastError` cleared — rather than a second Job being created
   under the same key. `Attempts` resets because this is a *new
   client-initiated attempt at the operation*, distinct in kind from the
   queue's own internal, automatic transient-failure retry budget
   ([0019](0019-retry-pipeline-granularity.md)) that governs attempts
   within a single activation of a Job; treating a deliberate client retry
   as if it were one more automatic transient-failure attempt would let a
   Job exhaust its entire attempt budget after only one or two
   client-visible retries, which is not what either mechanism is for. This
   keeps a single, stable Job identity per key for the life of that key —
   the alternative (minting a new Job per retry, all sharing one key)
   leaves a key's lookup ambiguous about which Job is authoritative at any
   moment, which this ADR treats as a design smell to avoid rather than
   resolve with a tiebreak rule.

   This introduces one new state-machine transition beyond what
   `docs/adr/0003-print-queue.md` describes: `failed → pending`, gated
   specifically on a same-key client retry (never triggered any other
   way). It is documented here, at the ADR that introduces it, rather than
   by editing `0003` itself — per this project's convention of not editing
   history away once an ADR is accepted.

## Consequences

- A client that participates (sends a stable `Idempotency-Key` across
  retries of what it considers one logical print) gets a real guarantee:
  at most one Job, and therefore at most one attempted physical print,
  regardless of how many times the HTTP call is retried, for 24 hours.
- A client that does not participate gets exactly today's behavior — this
  is opt-in, not a universal safety net. That is a deliberate, disclosed
  trade-off (see "Client-supplied key" above), not a gap this change
  introduces; it is strictly no worse than the status quo. It also does
  not by itself resolve the duplicate-print risk
  [0017](0017-queue-lifecycle-crash-recovery.md) leaves open for a
  crash-orphaned Job reconciled back to `Pending` — that risk is only
  closed for a client that supplied a key to begin with, since
  reconciliation isn't a client retry and has no key to check against.
- A Job's schema gains one optional field, and the store's contract gains
  one lookup capability, in addition to whatever
  [0016](0016-queue-concurrency-per-printer-workers.md) already added for
  claiming. Every existing lookup, save, and listing behavior is
  untouched.
- The idempotency-key lookup's cost grows with the number of stored Jobs,
  on every keyed print request, for whichever implementation each storage
  backend chooses. Accepted without profiling for v1, flagged as the first
  place to look if a real deployment's print latency ever regresses
  noticeably as its Job history grows — likely entangled with whatever
  [0017](0017-queue-lifecycle-crash-recovery.md) eventually decides about
  pruning old Job rows in the first place.
- The 24-hour retention window is a hardcoded value, not a config knob,
  for v1 — an operator who needs a different window has to change code,
  not config, until that need is demonstrated.
- A `Failed` Job retried under its original key is mutated back to
  `Pending` in place (with `Attempts` reset to zero), rather than
  replaced. Any code that assumed a Job's state only ever moves forward
  through `pending → running → {done, failed}` (plus `pending →
  cancelled`) now has one additional legal transition, `failed →
  pending`, gated specifically on a same-key client retry.
- The idempotency-key match is exact on the key string; nothing about
  this design hashes, normalizes, or truncates a client-supplied key — an
  operator/client is responsible for the key actually being stable and
  unique to the logical operation it represents, the same way Stripe's own
  convention leaves key generation to the caller.

## Alternatives considered

- **Dedup on Receipt content** (hash Receipt + printer name, no client
  header at all): rejected — this is the "too aggressive" failure mode
  this ADR opens with. Two customers legitimately ordering one identical
  item each must both get their receipt; content equality is not evidence
  of duplicate client intent, only a client-supplied key is.
- **Requiring `Idempotency-Key` on every print request**: rejected for
  v1 — a breaking change on rollout day for every existing client and
  template convenience endpoint, none of which have a key to send without
  their own separate change. Optional-with-safe-default preserves
  today's behavior for anyone not opting in.
- **A generic HTTP-response-cache layer** (middleware caching raw
  `(method, path, Idempotency-Key)` → response bytes, independent of the
  queue's own storage): rejected — this duplicates persistence Receiptd
  already has, with its own separate expiry policy to keep in sync with
  the Job's own lifecycle. It's a second, parallel mechanism for something
  the Job/store machinery already does, the same category of thing
  `CLAUDE.md` flags as worth raising rather than adding quietly (there,
  about extension mechanisms; the reasoning generalizes to persistence
  mechanisms). Enqueue-time Job dedup gives the same client-visible
  outcome — the identical Job ID comes back on retry — using the store
  that already exists.
- **A new Job per retry, all sharing one idempotency key, resolved by
  "most recent"**: rejected — leaves a key's lookup ambiguous about which
  Job is authoritative at any given moment, grows without bound for a
  client that retries a failing request repeatedly, and weakens the "one
  key, one Job identity" model this ADR is built on.
- **An active background sweep to expire or delete idempotency keys**:
  rejected in favor of lazy, comparison-only expiry with no deletion at
  all — no scheduler, no background process, nothing to get wrong on
  shutdown (deliberately [0018](0018-graceful-shutdown.md)'s scope, not
  this ADR's), and it cleanly separates "is this key still valid for
  matching" from "should this Job row still exist," which is
  [0017](0017-queue-lifecycle-crash-recovery.md)'s decision to make, not
  this one's.
- **A dedicated `KindConflict`/409 for a reused key with a different
  Receipt**: rejected — `docs/adr/0005-error-handling.md` deliberately
  keeps the `Kind` enum small and closed; this case is exactly "the
  request itself was the problem," which `KindValidation`/400 already
  covers, and does not justify a sixth `Kind` for one narrow situation.
- **Treat a same-key retry of a `Failed` Job as one more automatic
  transient-failure attempt (increment `Attempts` rather than resetting
  it)**: rejected — conflates a deliberate, client-initiated retry with
  the queue's own internal backoff/retry budget
  ([0019](0019-retry-pipeline-granularity.md)), which would let a Job
  exhaust its entire attempt budget after only a couple of client-visible
  retries and effectively defeat the point of offering a stable
  idempotency key at all.
