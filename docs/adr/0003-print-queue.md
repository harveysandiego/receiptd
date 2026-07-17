# 0003. Asynchronous, persistent print queue from day one

Status: Accepted

## Context

A print request can fail for reasons entirely outside the caller's
control: the printer is powered off, asleep, out of paper, or briefly
unreachable over the network. If `POST /api/v1/print` synchronously waited
on the physical print, the API would be as unreliable as the printer's
network connection, and a slow or offline printer would block the calling
client (or an automation depending on it) for however long a retry policy
takes to give up.

This could have been deferred — ship a synchronous v0.1 that prints
immediately and add a queue later once the need is proven. That path was
considered and rejected: retrofitting async behavior onto an API whose
contract is "the print already happened by the time this call returns"
would be a breaking change to that contract, not an additive one.

## Decision

Printing is asynchronous from the first version that talks to a real
printer. `app.Service.Print` validates the Receipt, resolves the target
printer's `Profile`, constructs a `queue.Job`, and enqueues it — returning
a job ID immediately, before the printer has been touched. A background
queue worker processes jobs independently of the HTTP request that
enqueued them.

The queue is backed by `bbolt` by default (`queue.NewBoltStore`) so job
state survives a restart, with an explicit in-memory opt-out
(`queue.NewMemoryStore`) for tests and for deployments that don't need
persistence. Job states are `pending → running → {done | failed}`, plus
`pending → cancelled` (a `running` job cannot be cancelled — it can't be
un-printed). Retries are bounded (default 3 attempts) with exponential
backoff, and — per `docs/adr/0005-error-handling.md` — gated strictly on
`apperr.KindTransient` failures; anything else fails the job immediately
with no retry.

`Queue` itself is a concrete struct, not an interface — there is exactly
one implementation and no present reason to swap it. `Store` is the
interface, because bbolt vs. in-memory is a real, present choice.

## Consequences

- API callers get an immediate, reliable response (a job ID) regardless of
  printer state, and can poll `GET /api/v1/jobs/{id}` for outcome.
- A temporarily offline printer doesn't lose queued jobs — they're
  retried, and survive a `receiptd` restart because the queue is
  bbolt-backed by default.
- The system has more moving parts on day one of real-printer support
  (Milestone 3) than a synchronous design would — a background worker,
  job state machine, and persistence layer all exist before the first
  physical receipt prints. This was accepted as the right trade because
  retrofitting async behavior later would be a breaking API change, not an
  additive one.
- Retry logic must correctly distinguish transient from permanent failures
  (see `docs/adr/0005-error-handling.md`) — getting this wrong either
  wastes retry budget on unrecoverable errors or gives up on genuinely
  transient ones. This is covered by dedicated `queue` tests using a fake
  `Store` and `Processor` with scriptable `apperr.Kind` failures.

## Alternatives considered

- **Synchronous printing in v0.1, queue added later**: rejected — see
  Context; this would be a breaking API contract change once added.
- **A message-broker-backed queue (e.g. an external Redis/RabbitMQ
  dependency)**: rejected as disproportionate to the problem and in direct
  conflict with the project's single-static-binary, minimal-dependency,
  Raspberry-Pi-friendly goals. `bbolt` is a pure-Go embedded store with no
  external process to run or configure.
- **SQLite (via `modernc.org/sqlite`, pure-Go, no cgo) for the job
  store**: noted as a viable future option if richer querying (filtering
  jobs by printer, date range, etc.) is ever needed, but not chosen for
  v1 — `bbolt`'s simpler key/value model is sufficient for the current
  `Store` interface (`Save`/`Get`/`List` with a basic `Filter`), and adding
  SQL only when a real query need appears follows the same "don't build
  ahead of a proven need" principle applied elsewhere in this project.
- **`Queue` as an interface**: rejected — there is exactly one
  implementation and no variation to abstract over; see
  `docs/ARCHITECTURE.md` §2 and §11 for the general principle this
  follows.
