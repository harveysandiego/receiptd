# 0021. TLS termination is a deployment concern, not an application one

Status: Proposed

## Context

`cmd/receiptd/daemon.go` starts exactly one `http.Server` and calls
`ListenAndServe` — there is no TLS certificate loading, no ACME/Let's
Encrypt client, and no `ListenAndServeTLS` anywhere in this codebase.
`internal/auth` (Bearer/Basic middleware, per `docs/adr/0005-error-
handling.md`'s `KindUnauthorized`) is the only credential-checking layer
Receiptd has. This gap between "no transport encryption" and "has request
authentication" has come up repeatedly enough in review that it needs a
recorded decision rather than being re-litigated each time: is the missing
TLS support an oversight to fix, or an intentional boundary?

It's intentional. Receiptd's stated deployment target — a Raspberry Pi or
small homelab server, run under systemd or Docker
(`CLAUDE.md`; `docs/adr/0018-graceful-shutdown.md`) — is a target where
operators overwhelmingly already run a reverse proxy (Caddy, nginx,
Traefik, SWAG, HAProxy, Apache) in front of every HTTP service they expose,
specifically because that proxy layer already solves certificate
issuance, renewal, and routing for every other service on the same host.
Certificate lifecycle management is a genuinely hard, ongoing operational
problem (ACME account state, renewal scheduling, revocation, multi-domain
SANs) that these tools solve well, are already deployed for other reasons
in the target environment, and are maintained by people whose entire
project is that problem — not a print-queue daemon's.

## Decision

**Receiptd intentionally does not terminate TLS.** It speaks plain HTTP
only, on the one listener `cmd/receiptd` starts. The supported
architecture delegates TLS termination to a reverse proxy that owns the
public-facing edge.

Everything that follows from "own the public-facing edge" is explicitly a
deployment concern, not something this codebase takes responsibility for:
TLS certificate issuance and management (including ACME/Let's Encrypt),
certificate rotation, HTTP/2, HSTS and other security headers, and
optional mutual TLS (client certificate auth at the edge). A reverse proxy
already does all of this well; Receiptd reimplementing any part of it
would be maintaining a second, worse copy of infrastructure this project
has no comparative advantage building, for a problem already solved in
every realistic deployment of this daemon.

What Receiptd *does* keep is everything at or above the application
layer: `internal/auth`'s Bearer/Basic request authentication, and any
future authorization logic. A reverse proxy terminating TLS is not a
substitute for Receiptd checking who is allowed to call `/api/v1/print` —
those are different layers solving different problems, and this decision
does not blur that line. An operator exposing Receiptd beyond a fully
trusted network is expected to run both: the proxy for transport security,
`internal/auth` for request authorization.

## Consequences

- Direct Internet exposure of `receiptd` itself, with no reverse proxy in
  front of it, is **not a supported deployment model**. Doing so means
  every request — including whatever credential `internal/auth` checks —
  travels in plaintext, and the daemon has no certificate or HSTS story of
  its own to fall back on. The README/deployment docs need to state this
  plainly, not leave it to be discovered by an operator who exposes a port
  without reading the fine print.
- On a local, fully trusted network (a homelab LAN, a Docker network with
  no external exposure), an operator may reasonably choose plain HTTP with
  no reverse proxy at all — that's a legitimate deployment shape this
  decision doesn't forbid, just one where transport encryption was never
  needed in the first place, not one Receiptd secures on the operator's
  behalf.
- The daemon's own surface area stays small: no certificate files to
  manage in `config.yaml`, no renewal timers to run inside `receiptd`
  itself, no TLS-library dependency, no interaction between a shutdown
  sequence and in-progress certificate renewal to reason about
  (`docs/adr/0018-graceful-shutdown.md` stays unaffected by this decision).
- Every deployment guide for Receiptd needs to show a reverse-proxy example
  (at least one of Caddy/nginx/Traefik) as the primary documented path, not
  an afterthought — since it's the actual supported way to expose this
  daemon beyond a trusted network.
- If a future requirement genuinely needs Receiptd to terminate TLS itself
  (an embedded deployment with no proxy available, a compliance
  requirement for in-process encryption, etc.), that is a new decision to
  justify on its own merits in a new ADR — not something to add quietly on
  the assumption that this one was an oversight. This ADR's existence is
  itself the record that it wasn't.

## Alternatives considered

- **Embed TLS support directly** (`ListenAndServeTLS`, operator supplies
  cert/key paths in `config.yaml`). Rejected — it would only ever be a
  weaker version of what a dedicated reverse proxy already does (no ACME
  automation without also embedding an ACME client, no renewal scheduling,
  one more thing to get wrong on a homelab box), for a capability nearly
  every realistic deployment of this daemon already has available at the
  proxy layer.
- **Embed ACME/Let's Encrypt support** (e.g. `autocert`), so `receiptd` can
  self-manage a public certificate with no proxy at all. Rejected for the
  same reason `docs/adr/0003-print-queue.md` rejected an external broker
  and `docs/adr/0016-queue-concurrency-per-printer-workers.md` rejected
  distributed coordination: disproportionate infrastructure for this
  project's actual deployment target, where a proxy already provides it.
  It would also require `receiptd` to bind port 443 and handle
  HTTP-01/TLS-ALPN-01 challenge traffic directly, at odds with running
  behind any proxy at all on the same host.
- **Require a reverse proxy and refuse to start without one detected**:
  rejected — there's no reliable way for `receiptd` to detect "a proxy is
  in front of me," and a trusted-LAN-only deployment with no proxy is a
  legitimate use case this decision explicitly permits, not a
  misconfiguration to reject.
