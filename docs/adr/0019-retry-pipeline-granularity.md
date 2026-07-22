# 0019. Retry the whole render→encode→send pipeline, not just Send

Status: Proposed

## Context

`app.Service.Process` (`internal/app/process.go`) runs three stages for
every `queue.Job`: resolve the `printer.Profile`, `render` (`layout.Build`
+ `canvas.Paint`), `escpos.Encode`, then `Printer.Send`. `queue.Queue`'s
retry loop (`internal/queue/process.go`, `ProcessNext`) calls the whole of
`Process` again on every attempt — up to `q.maxAttempts` times, with
exponential backoff between attempts — whenever the previous attempt
failed with `apperr.KindTransient`. There is no field on `queue.Job`
(`internal/queue/job.go`) that could hold a rendered `Canvas` or encoded
ESC/POS byte slice between attempts, and nothing in `Process` or
`ProcessNext` skips render/encode on attempt 2+. This is today's actual,
shipped behavior, not a hypothetical — this ADR is deciding whether to
change it.

Two options were evaluated:

- **Option A (current behavior)**: `Receipt → Render → Encode → Send`,
  the entire pipeline retried as one indivisible unit on every attempt.
- **Option B**: `Receipt → Render → Encode` once, then retry only `Send`
  on subsequent attempts, reusing the already-encoded bytes.

Three questions decide between them: does retrying render/encode redo
work that's expensive enough to matter on the target hardware (a
Raspberry Pi); does it risk producing a *different* result on a later
attempt than the one that was first validated; and what would Option B
cost to build and operate.

This ADR is scoped narrowly to that one question — how much of the
pipeline a retry attempt re-executes. It does not change, and is
compatible with, the unrelated changes
[0017](0017-queue-lifecycle-crash-recovery.md) and
[0016](0016-queue-concurrency-per-printer-workers.md) make to this same
retry loop for independent reasons (per-attempt `Attempts` persistence,
and reshaping the loop into a per-printer-scoped method, respectively).
Nothing below should be read as undoing either of those.

### Where a `KindTransient` can actually originate

Grepping every `apperr.Wrap` call site downstream of `Process` confirms
the queue's retry gate is doing exactly what it looks like it's doing:

- `render/layout` (`build.go`) wraps every one of its own failures —
  bad inline image data, an unsupported element type, a barcode/QR
  generation failure, a missing `assets.Store` — as `apperr.KindPermanent`.
  The one exception is a missing named asset, which surfaces whatever Kind
  `assets.Store.Get` itself chose (see below) — never `KindTransient`
  directly from `layout` itself.
- `render/canvas` (`paint.go`, `encode_png.go`) wraps every failure as
  `apperr.KindPermanent`.
- `render/escpos` (`encode.go`) wraps every failure as
  `apperr.KindPermanent`.
- `assets.Store` (`filesystem_store.go`, `memory_store.go`, `store.go`)
  wraps a missing name as `apperr.KindNotFound`, an invalid name as
  `apperr.KindValidation`, and an underlying I/O error (e.g. a permissions
  problem reading the file) as `apperr.KindPermanent`. None of these three
  is `apperr.KindTransient`.
- `printer` (`network.go`) is the *only* place in the whole call graph
  that produces `apperr.KindTransient` — a dial failure, a write-deadline
  failure, a write error, or a short write, all from `networkPrinter.Send`.

So the premise holds exactly: **only `Printer.Send` can plausibly fail
with `apperr.KindTransient`.** `queue.ProcessNext`'s retry loop only
spends retry budget on `KindTransient` (`apperr.Is(procErr,
apperr.KindTransient)`), so in the overwhelming majority of retry
sequences that actually occur in production, the render and encode stages
already succeeded on attempt 1 and are simply being redone, not
re-validated against anything that was in doubt.

### Is render/encode actually pure?

`render/layout.Build` and `render/canvas.Paint` are pure functions of
`(receipt.Receipt, printer.Profile, Font)` with one exception:
`receipt.Asset` elements are resolved by name through `assets.Store.Get`
at `layout.Build` time (`docs/ARCHITECTURE.md` §3 "Image vs. Asset";
confirmed in `render/layout/build.go`). `assets.Store` (both
`filesystemStore` and `memoryStore`) has no versioning, ETag, or
content-hash concept — `Put` simply overwrites whatever bytes were
previously stored under a name (confirmed by
`TestStore_PutOverwritesExisting` in `internal/assets/store_test.go`).
`escpos.Encode` is a pure function of `(*canvas.Canvas, printer.Profile)`
with no I/O of its own.

This means render+encode are deterministic *for a fixed asset store
state*, but the asset store's state is not itself fixed for the lifetime
of a Job sitting in the queue across a Send failure and a subsequent
retry — the store is shared, mutable, and has no relationship to the Job
that referenced it at enqueue time.

## Decision

**Keep Option A: retry the entire `Render → Encode → Send` pipeline on
every attempt, unchanged from today's behavior.** Do not add a cached
rendered/encoded byte field to `queue.Job`, and do not split retry into a
render-once/send-many-times shape.

This is a decision to leave `internal/app/process.go`'s and
`internal/queue/process.go`'s retry *granularity* exactly as it is — no
code change results from this ADR's own reasoning. (This is narrower than
saying those files never change: [0017](0017-queue-lifecycle-crash-recovery.md)
and [0016](0016-queue-concurrency-per-printer-workers.md) both touch the
same retry loop for their own, unrelated reasons — persistence frequency
and per-printer scoping, respectively. This ADR simply adds nothing further
on top of those, on the specific question of how much of the pipeline one
retry attempt re-executes.)

### Correctness: not a wash — Option A carries a real, if narrow, risk

If `Printer.Send` fails transiently on attempt 1 (job's `Receipt`
references `asset: logo.png`, render succeeds, encode succeeds, only the
network write fails), and between attempt 1 and attempt 2 something calls
`assets.Store.Put("logo.png", newBytes)` — a legitimate, unrelated
operation on a running system (asset management, per Milestone 4) — then
attempt 2's `render` produces a *different* `Canvas`, a *different*
encoded byte stream, and `Send` succeeds. The job reports `Done`, having
silently printed different pixels than what was rendered (and would have
been previewed) at attempt 1, with no error and nothing in `Job.LastError`
to indicate a mid-flight substitution ever happened. This is a genuine
correctness risk of Option A, not merely a performance cost — a receipt a
caller previewed, approved, and printed can, in a real if narrow race
window, come out physically different from what was approved.

The more common asset-mutation case — deletion, not replacement — does
*not* silently misbehave: `assets.Store.Get` on a missing name returns
`apperr.KindNotFound`, which is not retried, so the job fails outright on
attempt 2 with a clear, correct error rather than printing something
wrong. Only the *replace-in-place* case is silent.

This risk is real but narrow: it requires (a) a transient Send failure on
attempt 1, (b) an asset mutation landing in the specific window between
attempts, and (c) the job referencing a mutable named asset at all (a
`receipt.Image` embeds its bytes inline and cannot be affected — only
`receipt.Asset` is exposed to this). It does not apply to the common case
of a receipt with no `asset` elements, or to jobs that succeed or
fail-with-KindPermanent on the first attempt.

Would Option B eliminate this risk? Only partially, and only by
freezing exactly the wrong moment: Option B renders once, at whatever
point the code chooses to render-once — most naturally, still on attempt
1, before the first `Send` — and reuses those bytes for every subsequent
`Send` retry. That does make the *printed* result match whatever was
rendered at that one point, closing the specific race above. But it does
not change the fact that render/encode's pure-function guarantee only
holds "for a fixed asset store state" — Option B is choosing to trust
attempt-1's state as authoritative and never look again, rather than
Option A's "always re-check," and there is no evidence in this codebase
that attempt-1's state is more authoritative than attempt-2's; it is
simply *earlier*. Framed this way, Option B buys determinism-relative-to-
one-snapshot at the cost of a separate correctness question this ADR is
explicitly out of scope for (see "Alternatives considered"): *should a
Job's Receipt/rendering be frozen at enqueue time or at first-render time,
and does the eventual answer to that question want a persisted snapshot
rather than an in-memory one?* That is a strictly bigger design question
than "cache bytes across a retry loop," and this ADR does not attempt to
answer it.

### Performance: redundant work, but cheap at this project's actual scale

`render/layout.Build` and `render/canvas.Paint` operate on a single
receipt-sized `Document`/`Canvas` — this project's own hardware target
(`docs/ARCHITECTURE.md`'s Epson TM-m30II, 203 DPI, 72mm/576-dot printable
width) bounds the amount of work involved: a receipt is, by definition, a
short document (headings, text lines, a handful of QR/barcode/asset
elements), not a multi-page report. `escpos.Encode`'s own chunking logic
is explicitly a no-op today per `CLAUDE.md`'s performance philosophy and
`docs/adr/0002-raster-rendering.md`'s consequences section, which is
itself evidence the project doesn't consider this pipeline
performance-sensitive enough to have earned real profiling yet.

Redoing `layout.Build` + `canvas.Paint` + `escpos.Encode` on a retry is
therefore CPU work proportional to one small receipt's worth of glyph
painting and bitmap encoding — real cost, but the kind of cost this
project's own `CLAUDE.md` explicitly tells us not to design around
without a profile: *"Don't optimize for a guess... build the general
mechanism when the second concrete case shows up, not in anticipation of
it... profile first and let a benchmark justify the change."* No profile
exists showing re-render is a measured problem on a Raspberry Pi, and the
retry path itself is not the common path — it only executes at all when
`Printer.Send` has already failed once, which by `docs/adr/0003-print-
queue.md`'s own reasoning (temporarily offline/unreachable printer) is
expected to be an infrequent event relative to total jobs processed, not
a steady-state hot loop. Optimizing a cold path's CPU cost on unmeasured
hardware, at the expense of a permanent increase in `Job`/`Queue`
complexity, is precisely the trade `CLAUDE.md`'s performance philosophy
argues against. This holds regardless of how many printers
[0016](0016-queue-concurrency-per-printer-workers.md) has running
concurrently — each printer's retry loop redoes at most its own small
receipt's render/encode, not a shared or escalating cost.

### Determinism: mostly moot, with one caveat already covered above

If render+encode were *fully* pure functions of `(Receipt, Profile)` with
zero observable nondeterminism, Option A and Option B would be
behaviorally identical except for CPU cost, and this would purely be a
performance question with an easy answer (see above: don't optimize
against a guess). That is very nearly true here — the one gap is the
mutable, unversioned `assets.Store` lookup discussed under Correctness
above. This ADR does not manufacture a bigger determinism story than that
one real gap; the rest of the pipeline (layout math, glyph painting,
ESC/POS byte generation) has no clock reads, no randomness, and no other
I/O anywhere in `render/layout`, `render/canvas`, or `render/escpos` (confirmed
by inspection — no `time.Now`, `math/rand`, or file/network access appears
in any of those three packages' non-test source).

### Memory/persistence trade-off Option B would have introduced

Had Option B been chosen, `queue.Job` would need a new field to hold the
encoded byte slice across the retry loop's lifetime, and that field would
have to live somewhere:

- **`Queue`-level, in-memory only** (e.g. a `map[string][]byte` keyed by
  Job ID, held only for the duration of one `ProcessNext` call): cheapest
  to build, but the cache is already scoped to exactly one in-process
  `ProcessNext` call in today's design anyway (`ProcessNext` runs a Job's
  entire retry loop synchronously, in one Go call, before returning) — so
  in practice this reduces to a local variable inside `ProcessNext`, not
  a durable `Queue` field at all, and the crash-recovery question doesn't
  even arise: if the process dies mid-retry-loop, there was never
  anything to lose beyond what Option A already loses (an
  unfinished Job stuck in `JobRunning`, a question
  [0017](0017-queue-lifecycle-crash-recovery.md) owns).
- **`Job`/`Store`-persisted** (a `[]byte` field on `Job`, written to
  `bbolt` via `Store.Save`): this is the version that actually matters
  and actually costs something. `queue.store_bolt.go` JSON-marshals the
  *entire* `Job` on every `Save` call (attempt-transition to `Running`,
  and again at `Done`/`Failed`); a persisted encoded-bytes field would
  mean either (a) an extra `Save` call mid-retry-loop the first time
  bytes become available, or (b) accepting that a crash between "encode
  succeeded" and "Send eventually succeeds" loses the cached bytes anyway
  and must re-render on the next startup's recovery pass regardless — at
  which point Option B has bought nothing durable, only bought back the
  in-memory version's benefit for the crash-free case, at the cost of a
  measurably larger bbolt file for any receipt with embedded/asset image
  data (JSON-encoded base64 bytes, potentially the largest single field on
  a `Job`). This is exactly the "what if the daemon dies with
  encoded-but-unsent bytes cached" question
  [0017](0017-queue-lifecycle-crash-recovery.md) owns — this ADR flags it
  as a forward reference and does not resolve it.

Since even the cheap, in-memory version of Option B collapses to "a local
variable inside the function that already runs the whole retry loop in
one call" — buying nothing `ProcessNext` doesn't already have implicitly
— and the persisted version buys durability only by adding real,
measurable cost and reopening a question owned elsewhere, neither shape
of Option B clears the bar of "this problem is real and this is the
smallest fix for it."

## Consequences

- No code changes result from this ADR's own reasoning.
  `app.Service.Process` and `queue.Queue.ProcessNext` (or its
  [0016](0016-queue-concurrency-per-printer-workers.md)-scoped successor)
  keep retrying the full pipeline on every attempt, exactly as today.
- Every retry attempt continues to fully re-render and re-encode a Job's
  Receipt. On a printer that is offline for several retry cycles, this is
  real, repeated CPU work — accepted, per the Performance analysis above,
  until a real profile on target hardware shows it's actually a problem.
- The narrow correctness risk identified above (a `receipt.Asset`
  replaced in-place between a transient Send failure and the next retry
  silently changes what gets printed, with no error) is *not* fixed by
  this decision. It's called out explicitly here rather than hidden,
  because it's a real, if rare, gap: a future ADR could address it
  directly (e.g. resolving `Asset` content once at enqueue/first-render
  time and snapshotting it onto the Job) without needing to also solve
  Option B's broader cache-the-whole-pipeline-output problem — those are
  separable questions, and this ADR is deliberately not conflating them.
- `queue.Job`'s schema is untouched by this ADR — no new field, no
  serialization change, no bbolt file size growth attributable to retry
  granularity. This keeps [0017](0017-queue-lifecycle-crash-recovery.md)'s
  scope unaffected by this decision on this specific point: there is no
  new "what if we crash with cached bytes" case to add to that ADR
  because this decision creates no cached bytes. ([0020](0020-idempotent-print-requests.md)
  separately adds one small field, `IdempotencyKey`, unrelated to retry
  caching.)
- If a future profile on real Raspberry Pi hardware shows render+encode
  measurably delaying retry throughput (e.g. a printer that's offline for
  hours, with many jobs queued behind it, each burning CPU on every
  backoff cycle), that's the point to revisit this decision with real
  numbers — not before.

## Alternatives considered

- **Option B — render/encode once, retry only Send**: rejected as this
  ADR's Decision explains — no measured performance problem to justify it,
  the in-memory version of the cache is functionally a no-op given
  `ProcessNext`'s existing single-call-per-Job-lifecycle shape, and the
  persisted version costs real bbolt size and reopens the crash-recovery
  question for no durability benefit (a crash still loses the cached
  bytes in the common case). It also does not cleanly close the one real
  correctness gap identified (mutable asset content) — it only trades
  "trust attempt-2's state" for "trust attempt-1's state," an arbitrary
  choice of snapshot point, not a fix.
- **Snapshotting/resolving Asset content once at enqueue or first-render
  time, independent of the Option A/B choice**: this would close the
  narrow correctness gap directly (no later `assets.Store.Put` could ever
  change what a queued Job prints) without touching the render/encode/send
  retry granularity at all. Not adopted here because it's a `Job`/`Receipt`
  schema change orthogonal to this ADR's actual question (retry
  granularity) and deserves its own ADR if pursued — flagged in
  Consequences as a legitimate follow-up, not decided here.
- **Retrying only `Send` but re-encoding on every attempt (render once,
  encode+send together on retry)**: a middle option, briefly considered.
  Rejected for the same reason as Option B in full: `escpos.Encode` is
  cheap and pure, caching it separately from the `Canvas` buys negligible
  additional savings over Option B while adding a second cached artifact
  (`Canvas` and encoded bytes both) instead of one, for no additional
  benefit over just caching the final encoded bytes (i.e., Option B) or
  not caching at all (Option A).
