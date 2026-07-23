# 0017. Startup reconciliation for Jobs orphaned by a daemon crash while Running

Status: Accepted

## Context

`docs/adr/0003-print-queue.md` established the Job state machine this ADR
extends: `pending -> running -> {done | failed}`, plus `pending ->
cancelled`. That ADR is explicit that a `running` Job can't be cancelled
"because it can't be un-printed" — but it never addresses what happens to a
`running` Job when the *process* driving it disappears mid-flight, not just
when a caller asks to cancel it.

Today, nothing does. Walking the actual code confirms the gap precisely:

- `queue.ProcessNext` (`internal/queue/process.go`) transitions a Job to
  `JobRunning` and persists that transition with `q.store.Save` *before*
  entering its retry loop. It only persists again — to `JobDone` or
  `JobFailed` — after the entire loop (all attempts, including backoff
  sleeps) finishes. If the process dies anywhere between those two `Save`
  calls, the Job is left on disk with `State == JobRunning` permanently.
- `queue.Store.NextPending` (`internal/queue/store.go`,
  `store_bolt.go`, `store_memory.go`) only ever matches `State ==
  JobPending`. A Job stuck in `JobRunning` is invisible to it — the
  background worker will never pick it up again, by construction, not by
  bug.
- `cmd/receiptd/daemon.go`'s `runWorker` is a single background goroutine
  that calls `ProcessNext` in a loop. There is no startup reconciliation
  logic anywhere in `cmd/receiptd` today — `build`/`serve` wire the Queue
  and start the worker with no scan of existing state first.
- `runWorker` is started as `go runWorker(context.Background(), d.queue)`
  — its `ctx` is never cancelled by anything in this codebase today. There
  is no SIGTERM/graceful-shutdown handler at all yet
  ([0018](0018-graceful-shutdown.md) designs one). This matters for
  framing the problem correctly: **today, the only way a Job ends up
  `Running` with no live process behind it is a hard, unclean death** — the
  process being killed (`SIGKILL`, OOM-kill), a host power loss, or a Go
  runtime-fatal condition that `queue.callProcessor`'s `recover()` cannot
  catch (a stack overflow, a fatal error inside the `bbolt` driver, a panic
  outside `callProcessor`'s own call frame). An ordinary panic *inside*
  `Processor.Process` is already handled — `callProcessor` recovers it,
  logs it, and reports `apperr.KindPermanent`, which fails the Job
  immediately without touching the process. This ADR is about the class of
  failure that survives past that recovery, not the class it already
  covers.
- A subtler, load-bearing fact about the current code: `Job.Attempts` is
  only durable at the two points `ProcessNext` calls `Save` — once before
  the retry loop starts (where `Attempts` is unchanged from whatever it
  was) and once after the whole loop finishes. `next.Attempts++` inside the
  loop only mutates the in-memory `*Job`. A crash *during* the loop (mid
  `Process` call, or during a backoff sleep between attempts) leaves the
  persisted `Attempts` at its pre-call value, silently undercounting how
  many real attempts actually happened. This ADR's reconciliation decision
  depends on that count being trustworthy — see "A supporting requirement"
  below.
- A crash between a *successful* `printer.Send` and the final `Save` call
  that would have recorded `JobDone` is the sharpest version of the
  physical-world problem this ADR must be honest about: the receipt has
  already, actually printed, and the persisted state still says `Running`.
  Nothing in the Job record can tell the difference between that case and
  a crash that happened before `render/escpos.Encode` ever produced a
  byte.

### Scope boundaries

This ADR owns the Job lifecycle beyond what `0003` already defined:
orphaned `Running` Jobs, startup reconciliation, and the persistence
guarantees reconciliation depends on. It is deliberately narrow about
everything else:

- It does **not** design worker topology, printer concurrency, queue
  ownership, or job-claiming semantics — that is entirely
  [0016](0016-queue-concurrency-per-printer-workers.md)'s decision, made
  ahead of this one specifically so this ADR could build on a settled
  answer instead of hedging against an unknown one. This ADR only takes
  as given the one fact [0016](0016-queue-concurrency-per-printer-workers.md)
  commits to that matters here: every worker in the system lives inside a
  single OS process, and [0016](0016-queue-concurrency-per-printer-workers.md)
  permanently rules out more than one `receiptd` process sharing one
  store. Because of that single-process guarantee, plus this ADR's own
  requirement (below) that reconciliation completes before any worker
  starts, the rule "every Job found `Running` at daemon startup is
  unconditionally orphaned" holds — there is never a live worker of any
  kind for reconciliation to mistake for one. This ADR is the sole owner
  of that reconciliation rule and its reasoning; it is stated here once,
  not duplicated in [0016](0016-queue-concurrency-per-printer-workers.md).
- It does **not** design graceful shutdown (SIGTERM handling, draining an
  in-flight `Process` call cleanly). That is
  [0018](0018-graceful-shutdown.md)'s job. Whatever that ADR decides, the
  reconciliation described here still has to exist as the backstop for the
  *unclean* death case (`kill -9`, OOM, power loss) that no shutdown
  handler can ever fully prevent — this ADR is not superseded by, or
  redundant with, whatever the clean-shutdown path adds. In fact
  [0018](0018-graceful-shutdown.md) explicitly relies on this ADR's
  mechanism to resolve a Job it deliberately leaves non-terminal when a
  shutdown interrupts a retry's backoff wait.
- It does **not** design idempotency keys or any printer-side
  duplicate-suppression mechanism — that is
  [0020](0020-idempotent-print-requests.md)'s job. This ADR's Decision
  explicitly accepts a possible-duplicate-print outcome as a known,
  unresolved risk (see "Guarantees" below) rather than trying to solve it.
- It does **not** change queue pruning/retention policy. Reconciliation
  reads every persisted Job once at startup; that cost is discussed under
  Consequences, not resolved by adding retention.

## Decision

### The state machine is unchanged: five states, one new transition path

No sixth state is introduced. `JobPending`, `JobRunning`, `JobDone`,
`JobFailed`, `JobCancelled` remain exactly as `0003` and
`internal/queue/job.go` define them. What changes is that `JobRunning` gains
a second way out besides "the worker that put it there finishes it":
reconciliation can also move a `Running` Job to `Pending` (retry) or
`Failed` (give up), *on behalf of* a process that no longer exists to do so
itself. (A third, narrower way out — `failed -> pending`, gated on a
client-supplied idempotency key — is added separately by
[0020](0020-idempotent-print-requests.md); it does not interact with
`Running`, so it doesn't change anything decided here.)

### A reconciliation pass runs once at every startup, before any worker starts

At every daemon startup, before any worker
([0016](0016-queue-concurrency-per-printer-workers.md)'s per-printer
workers, or any future equivalent) begins claiming Jobs, the queue
performs a reconciliation pass: it scans every persisted Job, finds every
one still recorded as `Running`, and resolves each as described below.
This requires no new capability from the storage layer beyond what
already exists — a full scan of stored Jobs is already possible today, and
this ADR gives that capability a second real caller, not a new one.

For every Job found in `JobRunning`, the interrupted attempt counts
against that Job's retry budget exactly as an ordinary failed attempt
would — "the process died mid-attempt" consumes one unit of retry budget,
the same as any other failed attempt does in the normal retry loop. If
the Job's attempt count, with this one included, is still under its
configured maximum, the Job is returned to `Pending` with a fixed,
greppable diagnostic recorded as `LastError` (e.g. `"interrupted: daemon
restarted while this job was running (attempt N of M)"`) — it re-enters
the normal claiming rotation exactly like any other pending Job, with no
special-casing downstream. If the attempt budget is instead exhausted, the
Job moves to `Failed` with the same diagnostic. This is the terminal,
operator-visible signal the current code never produces: a Job that keeps
crashing the daemon (rather than merely failing inside `Process`) still
stops consuming restarts once its attempt budget is spent, and lands
somewhere an operator polling for a Job's status can actually see it,
instead of sitting invisibly in `Running` forever.

Reconciliation runs synchronously as part of daemon startup, strictly
before any worker begins claiming Jobs — not because of any race with
those workers (a `Running` orphan is invisible to claiming regardless of
ordering), but so that reconciliation is guaranteed complete before any
new job processing begins, and so the startup sequence is simple to
reason about and test as "reconcile, then serve." This scan naturally
finds every orphaned Job regardless of how many there are — under
[0016](0016-queue-concurrency-per-printer-workers.md)'s per-printer worker
model, a crash can leave up to one `Running` Job per configured printer,
not just one globally, and this decision handles that without
modification, since it was never written to assume at most one.
Reconciliation logs a single summary line (count of Jobs reconciled,
split retried vs. failed), consistent with how this package already logs
a recovered panic.

### A supporting requirement: `Attempts` must be durable per attempt, not only per call

The decision above — "was this the last attempt?" — is only correct if
`Job.Attempts` is durable across a crash that happens *during* the retry
loop, not just at its boundaries. This is not itself a second
architectural decision on equal footing with reconciliation; it is a
persistence precondition reconciliation depends on, called out
separately here because it changes existing write behavior and deserves
its own explicit justification. Concretely: whichever loop actually runs a
Job's retries under [0016](0016-queue-concurrency-per-printer-workers.md)'s
worker model must persist the Job (with its incremented `Attempts`)
immediately after each attempt, not only after the whole loop concludes.
This is the minimum change needed to make the undercount described in
Context stop happening; it does not change the retry/backoff decision
logic, the `apperr.KindTransient` gating, or the final `State`
transition — only when `Attempts` is written. This is a change to the
same retry loop [0016](0016-queue-concurrency-per-printer-workers.md)
reshapes into a per-printer operation — the two ADRs modify the same code
for independent, compatible reasons, not competing ones.

### Why retry (bounded), not always fail, and not always retry unconditionally

Given a `Running` orphan, the design has to pick a default: return it to
`Pending` (risking a duplicate physical print if the crash happened after
`printer.Send` succeeded), or send it straight to `Failed` (risking
under-retrying a crash that happened before anything was ever sent to the
printer, or before `layout.Build`/`canvas.Paint` even ran).

This project's own retry philosophy, already accepted in `0003` and
`0005`, is **at-least-once, not exactly-once**: `ProcessNext` already
retries any `apperr.KindTransient` failure — including the ordinary case
where `printer.Send` returns a transient error because the connection
dropped *after* some or all bytes reached the printer. Receiptd already
has no way to distinguish "the bytes never arrived" from "the bytes
arrived but the acknowledgment didn't" in that non-crash case, and it
already resolves that ambiguity by retrying, bounded by
`queue.max_attempts`. A crash while `Running` is the same ambiguity, more
severe in degree (the whole process is gone, not just one `Send` call) but
not different in kind. Treating it consistently — bounded retry against
the same `max_attempts` budget the operator already configured, rather than
inventing a second, crash-specific policy that behaves differently from
the existing transient-retry policy for what is fundamentally the same
"we don't know if it reached the printer" situation — was chosen because
it doesn't ask the operator to reason about two different retry policies
for what looks, from their side, like the same failure mode: the printer
either didn't get it, got it, or half-got it, and either way, `0003`
already decided this system's answer is "retry up to N times, then tell
me."

### Guarantees provided and not provided

Stated explicitly, since this ADR is centrally about what operators can
and cannot rely on:

**Provided:**

- A Job will never be left permanently invisible in `JobRunning` with no
  operator-visible path forward. Every daemon startup reconciles every
  `Running` Job it finds, into either `Pending` (another chance) or
  `Failed` (a visible, terminal, pollable outcome) — however many such
  Jobs exist, across however many configured printers.
- Delivery to the physical printer is **at-least-once**: a Job that
  Receiptd cannot confirm succeeded (whether due to an ordinary transient
  transport failure or a process crash) will be retried, up to
  `queue.max_attempts` real attempts, counting a crash-interrupted attempt
  as consuming one unit of that budget.
- A Job that exhausts its attempt budget — whether through ordinary
  transient failures, a crash, or a mix of both — always ends in `Failed`
  with an operator-inspectable `LastError`, never in silent limbo.
- **This holds regardless of how many printers are configured.** Every
  `Running` Job found at startup is reconciled, whether it's the only one
  in the system or one of several (one per printer) left by a crash under
  [0016](0016-queue-concurrency-per-printer-workers.md)'s worker model.

**Not provided:**

- **Not exactly-once.** Receiptd cannot tell, for a Job found `Running`
  after a crash, whether the physical printer received nothing, some
  bytes, all the bytes, or fully completed the print. Retrying such a Job
  can produce a duplicate physical receipt. This is an accepted,
  unresolved risk — not a bug this ADR leaves in by oversight, but a
  problem this ADR explicitly declines to solve, deferred to
  [0020](0020-idempotent-print-requests.md) (which itself only closes this
  gap for clients that opt in with an idempotency key — see that ADR's own
  "Not provided" framing).
- **Not print-order preservation across a crash.** `Job.ID` is random hex
  (`queue.newJobID`), not time-ordered, so claiming the lowest-ID pending
  Job is not "the oldest Job chronologically" even in the ordinary case
  (see [0016](0016-queue-concurrency-per-printer-workers.md)'s own note on
  this); reconciling a Job back to `Pending` does not give it any priority
  over Jobs enqueued after it, before or after the crash.
- **Depends on the single-process worker topology
  [0016](0016-queue-concurrency-per-printer-workers.md) commits to.** The
  rule "every `Running` Job found at startup is orphaned" is sound because
  reconciliation runs once, synchronously, before any worker starts, and
  because [0016](0016-queue-concurrency-per-printer-workers.md)
  permanently rules out more than one `receiptd` process sharing one
  store. It is a guarantee that depends on that decision holding, not a
  law independent of it — if this project ever revisits horizontal
  scaling across processes, this ADR's reconciliation logic would need to
  be revisited alongside it, since "found `Running` at this process's
  startup" would no longer imply "no other live process holds it."
- **Not a substitute for graceful shutdown.** This ADR does not attempt to
  avoid leaving Jobs `Running` in the first place —
  [0018](0018-graceful-shutdown.md) reduces how often that happens for a
  *clean* stop, but cannot eliminate it for an *unclean* one. This ADR is
  the backstop for whatever still gets through: the unclean crash, the OOM
  kill, the power loss, the bug a shutdown handler can't foresee — and also
  the backstop [0018](0018-graceful-shutdown.md) explicitly hands a Job to
  when it deliberately leaves one non-terminal mid-shutdown.

## Consequences

- The central problem this ADR exists to fix is fixed: a `Running` Job
  with no live process behind it no longer waits forever. It is either
  requeued and given another chance, or fails visibly, within one daemon
  restart of the crash that orphaned it.
- **Duplicate physical prints remain possible, and this ADR does not
  resolve that** — it only makes the existing at-least-once/duplicate-risk
  policy apply uniformly to the crash case instead of leaving the crash
  case as silent, unbounded limbo. A Job reconciled back to `Pending` may
  print again even though it already fully printed before the crash. This
  is an accepted, explicitly-documented risk, not a solved problem — true
  idempotency (e.g. so a printer-side duplicate can be detected or
  suppressed) is deliberately left to
  [0020](0020-idempotent-print-requests.md).
- The retry loop now performs one persistence write per attempt instead of
  one per call (i.e., up to `max_attempts` writes instead of one, for a
  Job that exhausts its retries). Against a per-Job key/value store this
  is a small, bounded cost (bounded by `queue.max_attempts`, which config
  already requires positive and typically single-digit) — accepted as a
  supporting requirement, because without it, reconciliation cannot make
  a correct decision about remaining retry budget after a crash mid-loop.
- Reconciliation adds one full scan of the entire Job history to every
  daemon startup. This capability already exists and already has this
  cost-shape; this ADR gives it a second real caller, not a new one. There
  is currently no pruning/retention policy (explicitly out of scope here),
  so this scan's cost grows with total job history, unbounded, for the
  life of a `receiptd` instance's data directory. This is the same shape
  of unbounded-linear-scan cost
  [0020](0020-idempotent-print-requests.md) separately accepts for its own
  idempotency-key lookup — both are flagged here together so neither reads
  as an isolated oversight: a future queue-pruning/retention ADR is the
  natural place to close both at once, not something either of these two
  ADRs needs to solve individually.
- A Job whose crash is caused by something that recurs deterministically
  on every restart (e.g. a Receipt that reliably OOM-kills the process,
  rather than one that merely fails inside `Process`, which
  `callProcessor` already handles without crashing) will still cost the
  operator up to `max_attempts` real daemon restarts before landing in
  `Failed` — each restart governed by whatever the process supervisor's
  (systemd, Docker, etc.) own restart policy and backoff are, not by the
  queue's in-process exponential backoff, since the crash already
  destroyed that in-process state along with everything else. This is
  slower and coarser than the ordinary in-process retry loop's backoff,
  and is an accepted consequence of recovery necessarily happening at
  process-restart granularity rather than in-process.
- `LastError` on a reconciled Job is overwritten with the fixed
  interruption diagnostic, which — combined with the `Attempts`-undercount
  problem this ADR otherwise fixes going forward — means whatever error (if
  any) the crashed attempt was actually experiencing before it died is
  lost. This is consistent with `LastError` already being an
  unstructured, overwritten-each-attempt string today (it was never an
  accumulating log), so this isn't a regression, just a limitation worth
  naming rather than silently accepting.

## Alternatives considered

- **Always fail an orphaned `Running` Job outright, never retry it**:
  rejected. It's simpler and never risks reprinting a Job that had already
  fully printed, but it systematically under-retries the (likely more
  common) case where the crash happened well before anything reached the
  printer — during config load, `layout.Build`, `canvas.Paint`, or
  `escpos.Encode`, none of which touch the printer at all. That would make
  a crash strictly worse than an ordinary transient network failure for
  the same underlying Job, which already gets retried up to
  `max_attempts` times under `0003`/`0005`'s existing policy — an
  inconsistency with no principled justification.
- **Always requeue to `Pending` unconditionally, with no `Attempts` bound
  applied to the crash case**: rejected. A Job that reliably crashes the
  daemon every time it's attempted would cycle `Pending -> Running ->
  crash -> Pending -> ...` forever instead of sitting in `Running`
  forever — marginally better (at least the daemon itself keeps running
  between cycles) but it reproduces the same "invisible and stuck" failure
  this ADR exists to close, just spread across restarts instead of frozen
  in one state. Bounding it against the existing `max_attempts` gives the
  same eventual, visible `Failed` outcome ordinary transient retries
  already have.
- **A new `JobInterrupted` state, requiring an explicit operator action
  (a new "retry" or "discard" API endpoint) to resolve**: considered, and
  rejected for this slice. It would introduce new API surface beyond this
  ADR's scope, and it's a materially bigger interaction-model change — every
  existing `JobState` switch (in `api`, in tests, anywhere a caller
  reasons about terminal vs. non-terminal states) would need a new case
  for a state that, in the common case, resolves itself automatically
  within one restart. If automatic reconciliation via `Attempts`/
  `max_attempts` proves insufficient in practice (e.g. operators want to
  manually force a suspicious Job to `Failed` without waiting out its
  budget), that's a reasonable future ADR, but it isn't required to close
  the specific gap (invisible, permanently-stuck `Running` Jobs) this one
  targets.
- **Query the printer for delivery/status before deciding retry vs.
  fail**: not seriously pursued, for the same reason
  `docs/adr/0015-printer-model-catalogue.md` rejected runtime
  auto-detection of printable width — there is no ESC/POS status query
  reliable and consistent enough across vendors for this project's
  single-transport `printer` package to depend on, and even a perfect
  status query wouldn't establish whether *this specific* raster job was
  the one received, which is squarely
  [0020](0020-idempotent-print-requests.md)'s problem, not something worth
  half-solving here.
- **A heartbeat/lease mechanism** (the in-flight worker periodically
  touches the Job's `UpdatedAt` so a restart can tell "still being actively
  worked by a live process" from "genuinely orphaned" apart from a simple
  state check): rejected for this slice as unneeded machinery for the
  problem as scoped. With
  [0016](0016-queue-concurrency-per-printer-workers.md)'s single-process
  rule, a `Running` Job found at the process's own startup cannot belong to
  any other still-live worker or process — there is none. A lease only
  earns its keep once more than one *process* can legitimately hold a Job
  at once, which [0016](0016-queue-concurrency-per-printer-workers.md)
  permanently rules out for this project; building it here would be
  solving a problem this codebase deliberately doesn't take on.
- **Keep `Attempts` persistence exactly as it is today (only at the retry
  loop's start/end) and have reconciliation treat every orphaned Job as
  "one attempt used" regardless of the persisted count**: considered as a
  smaller change that avoids touching the retry loop's write frequency.
  Rejected because it would make reconciliation's attempt bookkeeping
  actively wrong in the case that matters most for this ADR — a Job that
  crashes the daemon on, say, its third real attempt would have its
  persisted `Attempts` still reading whatever it was after the *second*
  call's final write, so incrementing "by one" from a stale base could let
  a Job consume more real restarts than `max_attempts` was ever meant to
  allow. Fixing the persistence granularity (the supporting requirement
  above) was judged worth the small added write cost rather than building
  reconciliation on top of a value already known to be unreliable.
