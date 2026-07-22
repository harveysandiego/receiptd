# 0018. Bounded graceful shutdown on SIGTERM/SIGINT, without interrupting an in-flight print

Status: Proposed

## Context

Today `receiptd` has no signal handling at all (`grep -rn "signal.Notify\|os/signal"`
returns nothing in this repository). `cmd/receiptd/daemon.go`'s `serve()`
starts the queue worker as a bare background goroutine seeded with
`context.Background()` — a context that is never cancelled by anything —
and then blocks on `d.srv.ListenAndServe()`. A `SIGTERM` or `SIGINT`
therefore falls through to `net/http`'s and the Go runtime's default
behavior: the process dies immediately, mid-request and mid-job, with no
chance for either the HTTP server or the queue worker to notice.

This matters more here than in a typical web service, for two reasons
specific to this project:

- **Physical hardware is on the other end of a `Job`.** `queue.Processor`
  (implemented by `app.Service`) drives `layout.Build` → `canvas.Paint` →
  `escpos.Encode` → `Printer.Send` for a `Job`. `Send` streams raster bytes
  to a stateful ESC/POS device over a TCP connection
  (`printer.NewNetworkPrinter`). There is no ESC/POS command in this
  design (per `docs/adr/0002-raster-rendering.md`) for "resume a partial
  raster image" — a connection dropped mid-`Send` is not resumable and can
  leave the printer holding a partially-fed, partially-cut, or garbled
  receipt. This is a materially different risk profile than killing an
  in-flight HTTP handler.
- **The deployment target is a Raspberry Pi / homelab, run under systemd
  or Docker** (`CLAUDE.md`; `Dockerfile`'s distroless/non-root/static
  build), not a Kubernetes cluster with readiness probes and `preStop`
  hooks doing traffic-shifting across replicas ahead of a pod's own exit.
  Both `docker stop` and `systemctl stop`/`restart` send `SIGTERM`, wait a
  configurable grace period, and then `SIGKILL` — an operator restarting
  `receiptd` after a config edit or a package upgrade is the routine case
  this ADR is written for, not an exceptional one.

Three pieces of groundwork already exist (or are decided in a sibling ADR)
that this decision builds on rather than duplicates:

- `internal/queue/process.go`'s `sleepCtx` (wired up as `Queue.sleep`) is
  already `context.Context`-aware and returns early on cancellation — added
  in a prior PR specifically so a future shutdown mechanism would have
  something to cancel into.
- `cmd/receiptd/daemon.go` already sets `ReadTimeout` (15s),
  `ReadHeaderTimeout` (5s), `WriteTimeout` (30s), and `IdleTimeout` (120s)
  on the HTTP server. These bound how long any single HTTP request or idle
  keep-alive connection may legitimately take *today*, independent of
  shutdown — they are cited below as data points for sizing a shutdown
  deadline, not something this ADR changes.
- [0016](0016-queue-concurrency-per-printer-workers.md), decided ahead of
  this ADR, replaces the single background worker goroutine with one
  goroutine per configured printer, known at startup from `buildPrinters`'
  map. This ADR is written directly against that topology — "the worker"
  below always means "each of those per-printer worker goroutines,"
  enumerated from the same map, not a single goroutine generalized later.

This ADR defines **only** the clean, signal-initiated shutdown path. What
happens to a `Job` found `Running` in the `Store` after an *unclean* death
(power loss, `SIGKILL`, a panic that somehow escapes
`queue.callProcessor`'s recover) is
[0017](0017-queue-lifecycle-crash-recovery.md)'s question, not this one's.
Where this ADR's own design produces a `Job` left in a non-terminal state
(see Decision), resolving that state at the next startup is deliberately
handed off to that same mechanism, not re-solved here. Idempotency keys are
not addressed here at all — see
[0020](0020-idempotent-print-requests.md).

## Decision

`receiptd` treats `SIGTERM` and `SIGINT` identically: both begin the same
graceful shutdown sequence, logged at the moment the signal is received
("shutdown signal received, draining..." or equivalent) so an operator
watching `journalctl -u receiptd` or `docker logs` can see the sequence
happen rather than just seeing the process disappear.

**Phase 1 — stop accepting new work, concurrently, immediately:**

- The HTTP server stops accepting new connections/requests immediately.
  A client attempting to open a new connection after this point sees
  connection-refused (or a reset), not a queued or slow-accepted
  connection — there is no grace window during which new requests are
  still let in.
- Every one of [0016](0016-queue-concurrency-per-printer-workers.md)'s
  per-printer workers stops attempting to claim a new pending Job once its
  current in-flight claim/process call returns. Any of those workers that
  is idle, in its poll sleep between claim attempts, has that sleep woken
  immediately rather than run out — no worker waits around doing nothing
  before it even starts draining.

These happen at the same instant and are not sequenced relative to each
other or to one another across printers: nothing in this design has the
HTTP layer wait on the queue, or one printer's worker wait on another's (a
`Print` request only enqueues and returns — per
`docs/adr/0003-print-queue.md`, it never blocks on any worker).

**Phase 2 — let in-flight work finish naturally, within a bound:**

- Any HTTP request already being handled at the moment shutdown began is
  allowed to run to completion and send its response normally — it is not
  aborted just because shutdown started. In practice, per
  `docs/adr/0003-print-queue.md`, a `/print` request is already fast (it
  enqueues and returns before the printer is touched), so the case that
  matters here is a `/preview` request still rendering/encoding a PNG.
- If a per-printer worker is, at the moment shutdown began, **actively
  executing a `Job`'s `Process` call** (rendering, encoding, or — the
  critical case — mid-`Send` to the printer), that call is **not
  cancelled**. Its context is left alone and it is allowed to run to
  completion (success or a natural failure of that one attempt).
  Deliberately: a `Send` in progress is a byte stream to physical
  hardware with no defined "abort safely" semantics, so treating it like
  an ordinary cancellable HTTP handler and cutting it off is not a safe
  default here — see Alternatives. This applies independently, per worker:
  one printer's worker finishing an in-flight `Send` never waits on, or
  is affected by, another printer's worker doing the same.
- If a per-printer worker is instead **sleeping in a retry's backoff wait**
  between attempts of the same `Job` (no data in flight — nothing physical
  is happening during a backoff wait), that wait **is** interrupted
  immediately rather than run out, reusing the ctx-aware `sleepCtx`
  machinery already built for this. Waiting out a full exponential backoff
  chain (5s, 10s, 20s at default config) purely to honor a retry schedule
  that an operator-initiated shutdown has already superseded buys nothing
  and only makes shutdown slower.
- A `Job` interrupted this way (cut short mid-backoff rather than either
  succeeding or exhausting `max_attempts` on its own terms) is **not**
  written back to the `Store` as `JobFailed`. Synthesizing a permanent
  failure for a retry the system chose not to wait for would misrepresent
  what happened — the `Job` simply did not get to finish its own retry
  policy. It is left in whatever non-terminal state it was already
  persisted in (`JobRunning`), and resolving a `Job` found `Running` at
  the next startup is exactly the case
  [0017](0017-queue-lifecycle-crash-recovery.md)'s reconciliation pass
  already has to handle for the crash scenario — this reuses that single
  mechanism rather than inventing a second one for the clean-shutdown
  case.

  **This is a deliberate change from today's behavior, not something
  ctx-cancellation already gives us for free.** Today, a backoff wait cut
  short by context cancellation falls through to the same code path as an
  exhausted retry loop and is persisted as `JobFailed` — this is currently
  unobservable only because nothing in the codebase ever cancels the
  worker's context (it runs with a background context that lives for the
  daemon's whole lifetime). Implementing this ADR therefore isn't just a
  matter of wiring the worker's context up to the shutdown signal — doing
  that alone would reproduce exactly the "mark it `Failed`" behavior this
  ADR rejects, just on a schedule that finally makes it observable. Making
  a cancelled backoff wait leave the Job non-terminal instead requires a
  corresponding change to that fallthrough, so cancellation and "retries
  exhausted" stop sharing one outcome.

**Phase 3 — a hard deadline, then exit regardless:**

- **The architectural commitment is that the drain (Phase 2) is bounded by
  a single internal deadline — shutdown always completes within a fixed,
  known bound, never waits indefinitely.** The specific value is an
  implementation parameter, not the decision itself: the implementation
  uses a bounded graceful-shutdown deadline, initially 30 seconds, chosen
  to match the existing `WriteTimeout` (the longest any single legitimate
  in-flight HTTP request, a PNG preview render, is already allowed to
  take) so that shutdown never cuts off a request the HTTP server itself
  would still be willing to finish. A typical `Job`'s render+encode+`Send`
  is expected to complete in low single digits of seconds, well inside
  that initial budget. The deadline applies once, to the whole drain
  across every worker and every in-flight HTTP request together — not per
  worker and not per request — so shutdown latency does not grow with the
  number of configured printers, whatever the deadline's value. If a
  `Send` is genuinely hung against an unresponsive printer with no timeout
  of its own (`printer.Printer.Send` has none today), this deadline is
  what eventually bounds the wait; see Consequences for why that's an
  honest gap, not a solved one. If real-world operation on a range of
  hardware shows the initial value is wrong, it is a constant to tune, not
  an architectural decision to revisit.
- If the deadline is reached before Phase 2's work has finished, the
  process exits anyway. Whatever `Job`s or HTTP requests were still in
  flight at that point are abandoned exactly as if the process had
  crashed at that instant — there is no special-casing to make a
  deadline-forced exit look different from an unclean death, because from
  the printer's point of view (and the `Store`'s) it genuinely isn't
  different; any `Job` left `Running` is picked up by
  [0017](0017-queue-lifecycle-crash-recovery.md)'s reconciliation on the
  next startup like any other orphan. This is logged clearly ("shutdown
  deadline exceeded, forcing exit") so it's distinguishable in logs from a
  clean drain, even though the resulting process exit looks the same to
  systemd/Docker.
- A second `SIGTERM`/`SIGINT` received while already draining (an operator
  hitting Ctrl-C twice, or re-issuing `docker stop`) is treated as an
  explicit request to skip the wait: the process logs that it received a
  repeat signal and exits immediately, same as a deadline expiry.

**Operator-facing requirement:** because Phase 3's internal deadline is
bounded (initially 30 seconds — see above), the external
`SIGTERM`→`SIGKILL` grace period configured around `receiptd` must be
comfortably larger than whatever that deadline currently is, or the
orchestrator's own kill will race — and likely preempt — this design's
own clean exit, silently turning every restart back into the abrupt case
this ADR exists to avoid. With an initial 30-second internal deadline,
that means a grace period on the order of 40 seconds — a margin large
enough for process teardown and log flushing, without being so large that
a stuck shutdown holds up a homelab reboot/upgrade for an uncomfortable
length of time. Concretely, at that initial value:

- **Docker**: the default 10-second grace period (`docker stop`'s
  default, and Compose's default `stop_grace_period`) is **too short** and
  must be raised — e.g. `docker stop --time 40 receiptd`, or
  `stop_grace_period: 40s` in a compose file.
- **systemd**: set `TimeoutStopSec=40s` (or higher) in the unit file;
  systemd's own default (90s) is already safe, but is worth setting
  explicitly rather than relying on the default going unnoticed.

The architectural commitment is "grace period must exceed the internal
deadline with a reasonable margin," not the specific numbers above — if
the internal deadline's value is ever tuned, the recommended grace period
moves with it, and deployment documentation needs to state whatever the
current pairing is explicitly, not leave it to be discovered the first
time a receipt comes out garbled after a restart.

## Consequences

- An operator running `docker restart`/`systemctl restart receiptd` after
  a config change no longer risks truncating a `/preview` response or, far
  more importantly, cutting an ESC/POS raster stream off mid-transmission
  and leaving a printer in a garbled or half-fed state. This was the
  primary problem this ADR exists to solve.
- The interruptible `sleepCtx` added in a prior PR finally gets a caller
  that uses its cancellation path for something real, rather than sitting
  unused until this ADR.
- Shutdown is no longer instantaneous. An operator forcing a quick restart
  may observe up to the full internal deadline (initially ~30 seconds) of
  "still draining" before the process exits, if something was genuinely
  in flight. This is a deliberate trade against the alternative (instant
  kill, unsafe for physical prints).
- Every deployment gets the same fixed internal deadline regardless of
  that operator's actual hardware (a Pi 3 driving a slow network-connected
  printer behaves differently than a Pi 5), and regardless of how many
  printers are configured. There is no config knob for this yet — see
  Alternatives for why that's deferred rather than solved now.
- This design is not self-contained: a `Job` cut short mid-backoff, or
  abandoned at the Phase 3 deadline, is left non-terminal in the `Store`,
  and this ADR explicitly relies on
  [0017](0017-queue-lifecycle-crash-recovery.md)'s reconciliation pass to
  define what happens to it at the next startup — the two ADRs are meant
  to be adopted together; this one is not complete on its own.
- `printer.Printer.Send` has no timeout of its own today. This ADR's
  internal deadline is the only thing that would eventually bound a truly
  hung `Send` against an unresponsive printer, and it does so by force-exiting
  the whole process rather than failing that one `Job` cleanly — a coarser
  outcome than a proper per-`Send` timeout would give. Flagged here as a
  real gap this ADR surfaces but does not fix; a dedicated `Send`-level
  timeout is a smaller, separate, and probably overdue change.
- Operators must actively raise their SIGTERM grace period above this
  design's defaults (Docker's 10s default is not safe to leave as-is).
  That's an easy step to forget, and forgetting it silently degrades every
  restart back to the abrupt-kill behavior this ADR is meant to replace —
  the README/deployment docs need to state the recommended grace period
  explicitly, not leave it to be discovered the first time a receipt comes
  out garbled after a restart.
- Shutdown now waits on however many per-printer workers
  [0016](0016-queue-concurrency-per-printer-workers.md) started, not a
  fixed one — the set of configured printers is already known at startup,
  so nothing here needs a separate runtime discovery step for "how many
  workers are there."

## Alternatives considered

- **Do nothing (today's behavior): no signal handling at all.** Rejected
  outright — this is the status quo this ADR replaces. It risks a garbled
  receipt on every restart that happens to land mid-print, which is
  unacceptable for a project whose entire premise is making the printer
  disappear as a concern; an operator restarting the daemon shouldn't be
  gambling with the paper currently in the printer.
- **Cancel the in-flight `Job`'s context immediately on signal, the same
  way an HTTP handler's request context gets cancelled.** Rejected — an
  ESC/POS raster send is a stateful byte stream to physical hardware, not
  an idempotent, resumable HTTP request. Cutting it mid-transmission can't
  be retried cleanly (there's no "resume this raster image" command,
  per `docs/adr/0002-raster-rendering.md`) and can leave the printer in a
  worse state (partial feed, partial cut, garbled raster) than simply
  waiting a few extra seconds for it to finish on its own.
- **Wait indefinitely for in-flight work, no internal deadline at all.**
  Rejected — an operator's tooling (`docker stop`, `systemctl restart`)
  already enforces its own outer bound and will `SIGKILL` regardless, so
  an unbounded internal wait doesn't add any real safety; it only removes
  this design's ability to log a clear "forced exit" reason and hand
  control back on its own terms, versus being killed by something outside
  the process with no chance to say why.
- **Let a `Job`'s retry backoff run out naturally during shutdown instead
  of interrupting it.** Rejected — nothing physical is happening during a
  backoff sleep, so there is no safety reason to wait it out, and doing so
  could add tens of seconds (5s+10s+20s at default config) to every
  shutdown that happens to catch a `Job` mid-retry, for no benefit. This
  is exactly the case the existing ctx-aware `sleepCtx` was built to
  handle.
- **Mark an interrupted mid-backoff `Job` as `JobFailed` immediately**
  (simplest to implement, and closest to what `ProcessNext`'s existing
  cancellation branch already does today). Rejected as the target
  behavior — it would misrepresent an operator-initiated shutdown as the
  `Job`'s own retry policy having been exhausted, which it wasn't. Left as
  a non-terminal state for [0017](0017-queue-lifecycle-crash-recovery.md)'s
  startup-recovery path to resolve instead, so there's exactly one
  mechanism (not two) for "what happens to a `Job` found `Running` that
  isn't actually still running."
- **Kubernetes-style `preStop` hook + readiness-gate draining.** Rejected
  as disproportionate to the stated deployment target
  (`CLAUDE.md`: Raspberry Pi / homelab, systemd or Docker, not a
  multi-replica cluster behind a load balancer doing traffic-shifting
  ahead of a pod's own termination). That mechanism solves a
  multiple-replica traffic-draining problem this single-instance design
  doesn't have.
- **A configurable shutdown-timeout field in `config.yaml`.** Not
  rejected outright, but deferred — the frozen config schema
  (`docs/ARCHITECTURE.md` §7) has no such field, and one fixed, documented
  default (following the same pattern the existing HTTP timeouts already
  set — sensible fixed constants rather than a knob nobody has asked to
  turn yet) is enough until real operational experience on a range of
  hardware shows the initial value is actually wrong for someone. Adding a
  config knob ahead of that would be building against a guess, not a
  proven need.
- **A second, independent timeout specifically for the queue workers'
  drain, separate from the HTTP deadline.** Rejected in favor of one
  shared deadline for both — introducing two numbers an operator has to
  reason about independently, when a typical `Job` is expected to finish
  in low single digits of seconds anyway, adds complexity the current
  evidence doesn't justify. If real-world `Job` durations turn out to need
  materially longer than the initial deadline on some hardware, that's the
  point to reconsider a separate, larger queue-specific bound — not now.
- **A per-worker deadline instead of one shared deadline across all
  workers.** Considered given
  [0016](0016-queue-concurrency-per-printer-workers.md)'s per-printer
  topology, and rejected: it would make total shutdown latency scale with
  printer count in the worst case (N printers each genuinely needing the
  full budget), which contradicts the goal of a bounded, predictable
  restart on a homelab system regardless of how many printers are
  configured. One shared deadline across the whole drain keeps the
  worst-case bound constant regardless of its specific value.
