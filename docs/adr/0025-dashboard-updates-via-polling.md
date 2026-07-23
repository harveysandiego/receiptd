# 0025. Dashboard reflects printer/job state via client-side polling, not a push channel

Status: Proposed

## Context

Milestone 4 (`docs/ARCHITECTURE.md` §10) adds `internal/webui` with a
dashboard showing "printer status, quick actions, preview, text printing,
image upload, asset management, printer settings." A dashboard an operator
leaves open in a browser tab needs some way to notice that a printer went
offline or a job finished, without the operator manually reloading the
page every time.

`printer.Printer.Status(ctx)` is a synchronous, on-demand reachability
check — `internal/printer/network.go`'s implementation dials a fresh TCP
connection per call and holds nothing open between calls. There is no
event, pub/sub, or notification mechanism anywhere in this codebase today:
`internal/queue`'s workers are polling/worker-based
(`docs/adr/0016-queue-concurrency-per-printer-workers.md`), and nothing
currently pushes state changes to a caller. Any dashboard-refresh design
has to either build a new push mechanism from scratch or have the browser
ask.

The deployment target constrains the answer further
(`CLAUDE.md`; `docs/adr/0021-transport-security-via-reverse-proxy.md`): a
Raspberry Pi or small homelab server, a handful of printers, and a handful
of concurrent browser tabs at most — not an internet-scale dashboard with
thousands of idle connections to hold open. `docs/adr/0021` also already
commits this project to sitting behind a reverse proxy for TLS
termination; any long-lived connection (SSE or WebSocket) has to traverse
that proxy correctly too, which is one more thing every deployment guide
has to get right, on top of the proxy's own timeout/buffering
configuration for non-request/response traffic.

## Decision

The dashboard learns of printer/job state changes by **periodic
client-side polling** against a status endpoint owned by `internal/webui`
itself (consistent with `docs/adr/0022-webui-server-rendered-html-template.md`:
no JSON round-trip through `internal/api` from the browser's own UI),
backed by `internal/app.Service` calling `printer.Printer.Status` and
reading current job state — no new server-side push infrastructure. This
choice complements `docs/adr/0022`'s server-rendered, request/response UI
model: a persistent push channel would need its own always-on connection
handling that a purely request/response frontend has no other reason to
carry. The browser's JS re-fetches the status endpoint on a timer and
re-renders whatever changed; the interval's exact value is a UI
implementation detail left to whoever builds the dashboard, not part of
this decision. Between polls, the page shows the last-fetched state; there
is no guarantee of sub-poll-interval freshness, and this decision accepts
that trade explicitly.

`internal/webui` calls into `internal/app.Service` for this data the same
way `internal/api` already does — no separate code path is introduced for
the dashboard's benefit. No new goroutines, no held-open connections, and
no new interaction with `docs/adr/0018-graceful-shutdown.md`'s bounded
drain: each poll is an ordinary, short-lived HTTP request that starts and
finishes well within existing `ReadTimeout`/`WriteTimeout` bounds, exactly
like every other request this daemon already serves.

## Consequences

- No new long-lived-connection handling in `cmd/receiptd`'s `http.Server`
  — every request the dashboard makes looks like any other request this
  daemon already serves and needs no new code in the graceful-shutdown
  path.
- The reverse proxy in front of Receiptd (`docs/adr/0021`) needs no
  special configuration for streaming/long-lived connections; ordinary
  request proxying already works for this.
- Dashboard state is only as fresh as the last poll interval — a printer
  going offline or a job completing is visible with a delay of up to one
  poll period, not instantly. For a homelab dashboard this is an
  acceptable trade, not a hidden regression, but it is a real, named
  limitation.
- A handful of open tabs each polling independently means a handful of
  extra requests every few seconds — negligible at this project's stated
  scale (a homelab Pi, a few printers, a few tabs), but this design does
  not scale to many simultaneous dashboard viewers; that's out of scope
  for the stated deployment target.
- If a future requirement genuinely needs sub-second push updates (not
  something Milestone 4's scope calls for), that needs its own ADR
  justifying the added infrastructure on its own merits — this decision
  should not be treated as a stepping stone toward one by default.

## Alternatives considered

- **Manual refresh only (no automatic re-fetch at all).** Rejected — a
  dashboard an operator leaves open is exactly the case this doesn't
  serve well: the operator has to remember to hit reload to find out a
  print failed or a printer went offline. Cheap periodic polling costs
  little at this project's scale and removes that burden; plain manual
  refresh is a worse trade, not a simpler one.
- **Server-Sent Events (SSE).** Rejected as disproportionate here: it
  requires the HTTP server to hold connections open indefinitely, which
  is a new category of "in-flight work" that `docs/adr/
  0018-graceful-shutdown.md`'s bounded drain design didn't account for
  (an SSE stream isn't a request that finishes on its own; shutdown would
  need new logic to decide when to cut it). It also has to traverse the
  reverse proxy correctly (`docs/adr/0021`), which is one more thing every
  deployment guide needs to get right for a benefit — modest freshness
  improvement over a short poll interval — this project's scale doesn't
  need. There is also no existing event/pub-sub mechanism in this codebase
  to feed an SSE stream from; one would have to be built for this alone.
- **WebSockets.** Rejected for the same core reasons as SSE, only more so:
  full bidirectional connection state, its own library dependency, the
  same graceful-shutdown and reverse-proxy interaction concerns, and
  still nothing in this codebase today to push events from. Nothing about
  "printer status, quick actions, preview" (the Milestone 4 scope) needs
  the client to push data over the same channel — the dashboard only ever
  needs to ask, so a bidirectional channel buys nothing polling doesn't
  already deliver.
