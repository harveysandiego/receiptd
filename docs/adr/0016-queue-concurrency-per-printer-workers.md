# 0016. Per-printer worker concurrency and atomic Job claiming

Status: Proposed

## Context

Receiptd's stated future requirement is that multiple configured printers
must be able to print **simultaneously** — a slow or offline printer must
never hold up printing on a different, healthy printer. Today's
implementation does not provide that, and the gap is real rather than
hypothetical: `config`'s frozen schema (`docs/ARCHITECTURE.md` §7) already
documents more than one `printers:` entry (`front-desk`,
`custom-printer`), `cmd/receiptd/daemon.go`'s `buildPrinters` already
constructs a `map[string]printer.Printer` keyed by printer name for
however many are configured, and several tests already exercise two
distinct `PrinterName` values (`internal/app/service_test.go`,
`internal/app/job_status_test.go`). Multiple printers are not a
speculative future case this ADR is getting ahead of — they're a shape
the config surface, and this codebase's own tests, already commit to.

What hasn't caught up is the worker. `cmd/receiptd/daemon.go`'s
`runWorker` starts exactly one background goroutine for the entire
daemon, which calls `queue.Queue.ProcessNext` in a loop regardless of how
many printers are configured. `queue.Store.NextPending` (`internal/queue/
store.go`) returns "the first Job (ordered by ID) with State ==
JobPending" — globally, with no awareness of `Job.PrinterName`
(`internal/queue/job.go`). `ProcessNext` (`internal/queue/process.go`)
processes exactly one Job per call, including that Job's full retry loop
(bounded attempts, exponential backoff, gated on `apperr.KindTransient`
per `docs/adr/0003-print-queue.md` and `docs/adr/0005-error-handling.md`).
Put together: if the oldest pending Job belongs to a printer that is
offline, that one goroutine spends its entire time retrying and backing
off on that Job, and every other Job — including ones queued for a
completely healthy, idle printer — waits behind it. This isn't a
theoretical risk either: `internal/printer/network.go`'s own doc comment
on `networkTimeout` already names it explicitly — "the queue worker
processes one Job at a time ... so one such printer would wedge the
entire print queue indefinitely, not just its own Job."

Two further facts about the current code make this the right point to
also settle claiming and same-printer safety, not just cross-printer
fairness:

- `NextPending` is **not** an atomic claim. It just reads the current
  state; `ProcessNext` separately flips the returned Job to `JobRunning`
  and calls `Save` afterwards. Nothing prevents two concurrent callers
  from both reading the same Job as Pending before either has saved it as
  Running — this doesn't manifest today purely because there is exactly
  one caller in the whole process. Introducing more workers without
  closing this gap would introduce a live double-processing bug, not
  reveal a latent one.
- `printer.Printer`'s `networkPrinter` implementation (`internal/printer/
  network.go`) dials fresh per `Send` call and holds no internal mutex or
  connection pool. The fact that it's never called concurrently today is
  an accident of there being one worker in the daemon, not a designed
  property of `networkPrinter` itself. Physically, a single ESC/POS
  thermal printer processes one raster stream over one connection at a
  time; two concurrent `Send` calls to the same physical printer would
  race to write to it and produce interleaved, garbled output. Whatever
  this decision does, it must make "never two concurrent Sends to the
  same printer" a designed invariant, not a coincidence.

This project's deployment target constrains the shape of the answer:
a Raspberry Pi or small homelab server, `bbolt` or in-memory storage, and
"a handful of printers," not hundreds (CLAUDE.md, `docs/adr/
0003-print-queue.md`). Per CLAUDE.md's "discover interfaces at the second
real use" and "don't build the general mechanism before a second concrete
case shows up," this ADR does not reach for a distributed-systems-grade
answer (an external message broker, leader election, a lock service) —
`docs/adr/0003-print-queue.md` already rejected an external broker for
the same reasoning, and that reasoning applies here without modification.

Explicitly out of scope for this ADR, per the project owner: redesigning
`Store.Save`/`Get`/`List`'s signatures; redesigning global (cross-printer)
FIFO ordering; idempotency keys (see
[0020](0020-idempotent-print-requests.md)); graceful shutdown (see
[0018](0018-graceful-shutdown.md)); and crash recovery of Jobs stuck in
`JobRunning` (see [0017](0017-queue-lifecycle-crash-recovery.md)). This
ADR is being decided first, ahead of those three, precisely because each
of them needs to know the worker topology this ADR settles. In
particular, **this ADR owns the worker topology and claiming invariant
only** — it does not describe what happens to a Job left `Running` by a
dead worker, or how or when startup reconciliation runs. It states only
that a reconciliation step exists and completes, for whatever it decides
to do, before any worker described below starts; the reconciliation
decision itself belongs entirely to
[0017](0017-queue-lifecycle-crash-recovery.md).

## Decision

**One worker per configured printer, not one global worker.** Receiptd
runs one independent worker of execution per configured printer name —
today's single background goroutine is replaced by one per entry in the
configured printer set, each responsible only for that printer's Jobs.
With today's typical homelab deployment (one configured printer), this is
exactly one worker, doing exactly what the daemon does today: no behavior
change for the common case. The difference only appears once a second
printer is configured — which, as Context establishes, is already a
supported and tested shape, not a speculative one. This is why the
decision is to build this now rather than defer it: the "second real
case" this mechanism serves already exists in the frozen config schema.
This remains true for the whole daemon lifetime of one OS process — see
"Permanently out of scope" below.

**Per-printer drain, one physical store.** The persistence layer stays one
physical store — no per-printer sharding of storage, and no change to the
existing lookup-by-ID, save, or list operations. What changes is how a
worker decides what to process next: **the queue must provide an atomic
operation that claims the oldest pending Job belonging to one specific
printer**, transitioning it to the running state as part of that same
atomic step. No two concurrent callers of this operation — for the same
printer or for different ones — may ever observe the same Job as
claimable and both mark it running. This is the architectural commitment;
how each storage backend satisfies it (a single transactional write, an
exclusive lock spanning the read-decide-write sequence, or any other
mechanism appropriate to that backend) is an implementation choice, not
part of this decision. The existing "give me the next pending Job,
globally, regardless of printer" lookup is untouched and remains available
for any consumer that genuinely wants it (e.g., a future admin/diagnostic
view) — it simply stops being what a worker uses to decide what to claim.

**Cross-printer concurrency: yes.** Each printer's worker only ever claims
Jobs for its own printer, so printer A retrying with backoff can never
prevent printer B's worker from claiming and sending B's Jobs. This is the
mechanism that directly fixes the head-of-line blocking described in
Context.

**Same-printer concurrency: no — structurally, not by convention.**
Exactly one worker exists per printer name; two workers never drain the
same printer's lane. That means a printer's transport is only ever
invoked from one place at a time, by construction of the worker topology —
not because the transport implementation defends itself with a lock. This
follows the same principle CLAUDE.md states for Profile vs. Connection:
the serialization guarantee belongs to the layer that knows about printer
identity (the queue/worker layer), not to the transport itself, which
shouldn't need to reason about concurrent callers by design. The atomic
claim invariant above is defense in depth for this same guarantee, not its
primary source — it protects one specific Job from ever being
double-claimed even in a scenario this design doesn't otherwise create
(e.g., a future bug that accidentally starts two workers for one printer
name).

**Ordering guarantee, per printer.** Within one printer's lane, Jobs are
claimed oldest-first — the same ordering guarantee the daemon already
provides today, now scoped to one printer instead of applied globally.
Global FIFO across all printers (a Job for printer B running before an
earlier-enqueued Job for printer A, because B is idle while A is
retrying) is not guaranteed, and redesigning that is explicitly out of
scope per the project owner. Any caller depending on strict cross-printer
submission order must serialize its own requests; nothing in this design
promises it, and nothing about it was ever a deliberate guarantee before
now — it was an accident of "one worker, one ordering."

**What this settles for the ADRs that follow, stated as a fact this ADR
commits to, not as a description of what they do with it:**

- The worker topology is: one worker per configured printer, all within a
  single OS process, and a startup reconciliation step (owned by
  [0017](0017-queue-lifecycle-crash-recovery.md)) runs to completion
  before any of these workers starts. [0017](0017-queue-lifecycle-crash-recovery.md)
  is the sole owner of what that reconciliation step does and why.
- [0018](0018-graceful-shutdown.md) needs to wait for however many workers
  are running, not a fixed one — this ADR's answer is "one per configured
  printer," known at startup, so shutdown can enumerate exactly that set.
- [0020](0020-idempotent-print-requests.md) needs the same check-then-act
  atomicity discipline this ADR establishes for claiming, applied to a
  different operation (dedup-then-create at enqueue time rather than
  claim-for-processing). It reuses this ADR's invariant rather than
  inventing a second one.
- Per-printer concurrency means up to N Jobs (one per printer) can now be
  simultaneously mid-processing, instead of at most one Job daemon-wide.
  [0020](0020-idempotent-print-requests.md) is written with this in mind.
  [0019](0019-retry-pipeline-granularity.md)'s retry-loop analysis holds
  per-Job regardless of how many other Jobs (for other printers) are
  concurrently in their own retry loops — nothing about that ADR's
  reasoning assumed daemon-wide single-Job execution.

**Permanently out of scope: horizontal scaling across processes or
machines.** Running more than one `receiptd` process against one shared
store — whether on one machine or several — is not addressed by this
decision and is not a goal for this project's deployment target. A shared
bbolt file is not safely writable from two processes today (bbolt takes
an exclusive file lock on open), and this ADR does not change that.
Making multi-process operation safe would require real distributed
coordination (a lock service, leader election, or moving off bbolt
entirely) — exactly the class of complexity `docs/adr/
0003-print-queue.md` already rejected for this project's storage layer,
for the same reason: it is disproportionate to a single-Pi/homelab-server
system with a handful of printers. If multi-process operation is ever a
real requirement, it needs its own ADR built on its own justification, not
an extension of this one.

## Consequences

Good:

- A slow or offline printer no longer blocks Jobs queued for a different,
  healthy printer — this was the concrete problem forcing this decision,
  and it is fixed directly by giving each printer its own worker and its
  own claim scope, not by any heuristic layered on top of the existing
  retry/backoff state machine.
- Behavior for the common single-printer homelab deployment is unchanged:
  exactly one worker, exactly today's per-Job ordering, nothing new to
  observe. There is no migration risk for the deployment shape this
  project is actually built for on day one.
- The double-claim race that exists today only by accident (one caller,
  so it never triggers) is closed structurally, via the atomic claim
  invariant, before a second worker ever exists to trigger it — rather
  than being discovered as a live bug once multi-printer support actually
  ships.
- Printer transport implementations, present and future, never need to
  reason about concurrent send calls to themselves. That guarantee is
  owned entirely by the worker topology, consistent with this project's
  "capabilities and transport are different concepts" principle — a
  future USB or Bluetooth transport doesn't inherit a new locking
  obligation just because concurrency exists elsewhere in the system.
- This ADR's atomic-claim invariant is reused directly by
  [0020](0020-idempotent-print-requests.md) for a structurally similar
  check-then-act problem at enqueue time, instead of that ADR having to
  invent a second correctness discipline for the same storage backends.

Bad / costs:

- The daemon now runs N workers (one per configured printer) instead of
  one. For this project's stated scale ("a handful of printers") that is
  a handful of goroutines, not a scaling concern in itself — but it is a
  real increase in what shutdown has to wait for
  ([0018](0018-graceful-shutdown.md)) and in what crash recovery has to
  reason about: N workers can each leave one Job stuck in the running
  state at the moment of a crash, not just one
  ([0017](0017-queue-lifecycle-crash-recovery.md) owns how that's
  handled).
- Every storage backend — today's two, and any future one — must
  implement the atomic-claim invariant correctly. This is a correctness
  obligation of exactly the kind that produces rare, hard-to-reproduce
  bugs when subtly wrong: broken atomicity is invisible in normal
  single-printer operation and only surfaces under real concurrent
  claiming. `go test -race ./...` (CLAUDE.md, non-negotiable) is the main
  defense; the implementation should include a test that exercises many
  concurrent claim attempts — for the same printer and for different ones
  — against every storage backend, asserting no Job is ever claimed by
  more than one caller.
- Cross-printer global ordering, which happened to hold today only as a
  side effect of "one worker, one ordering," is now explicitly not a
  guaranteed property. Anything that implicitly relied on daemon-wide
  submission order (documentation, an operator's mental model, a client
  integration) needs to be corrected to "ordered per printer," not
  "ordered across the whole daemon" — a real, if narrow, behavior change
  to call out plainly rather than let it be discovered by surprise.
- The queue's processing surface grows to express "process the next Job
  for this printer" rather than "process the next Job, globally." Existing
  tests written against today's global, single-worker semantics need
  updating for the new shape. [0017](0017-queue-lifecycle-crash-recovery.md)'s
  persistence requirement for `Attempts` applies to whichever loop
  actually runs a Job's retries under this new topology — the two ADRs
  are changing the same loop for different, compatible reasons, not
  proposing two competing versions of it.

## Alternatives considered

- **Keep one global worker; accept head-of-line blocking.** Rejected —
  this is the status quo, and it directly contradicts the stated future
  requirement that multiple printers must be able to print
  simultaneously. A Job for a healthy printer must not wait behind a
  stuck retry loop for a different, offline one.
- **One global worker that skips a Job whose printer looks unhealthy
  (peeking at printer status before attempting it).** Considered and
  rejected: this still funnels every printer through one goroutine, so at
  best it avoids visibly wedging on an obviously-dead printer — it does
  not deliver genuine simultaneous sending to two healthy printers, which
  is the actual requirement. It would also introduce a second, heuristic
  notion of "is this printer OK to try right now" running alongside the
  retry/backoff state machine ADR-0003 and ADR-0005 already own — two
  mechanisms partially answering the same question is exactly the kind of
  drift CLAUDE.md warns against.
- **A fixed-size worker pool decoupled from printer count** (e.g. always
  4 workers pulling from the global queue). Rejected: sized larger than
  the printer count, nothing stops two pool workers from both landing on
  the same printer's Jobs, reintroducing the same-printer
  concurrent-send hazard this decision treats as a hard "no"; sized
  smaller than the printer count, it reintroduces head-of-line blocking
  with a bigger head. One worker per printer name is the only sizing rule
  that directly maps onto both "never two concurrent sends to one
  printer" and "one printer never blocks another."
- **Physically partition the store per printer** (a separate bucket or
  file per printer name). Rejected — a Job's printer name already lets one
  physical store answer "give me the next pending Job for printer X"
  without partitioning storage, and partitioning would complicate a Job-ID
  lookup (it would need to search N partitions, or take a printer-name
  hint it doesn't take today) for no benefit this design needs. This
  decision is about drain semantics, not physical storage layout, matching
  how the problem was framed.
- **Preserve global cross-printer FIFO via a shared claim-ordering
  ticket** (e.g. a monotonic sequence number workers take turns
  consuming). Rejected as out of scope: redesigning global ordering
  guarantees was explicitly excluded from this decision by the project
  owner. Per-printer ordering, as decided above, is sufficient for the
  stated simultaneous-printing requirement; nothing here forecloses a
  future ADR revisiting global ordering if a genuine need for it appears.
- **Horizontal scaling: multiple `receiptd` processes sharing one store,
  coordinated via a lock service or leader election.** Rejected outright
  for this project's deployment target — see the Decision's "Permanently
  out of scope" section. Disproportionate to a single-Pi/homelab-server
  system, and the same class of complexity `docs/adr/
  0003-print-queue.md` already rejected for the queue's storage backend.
