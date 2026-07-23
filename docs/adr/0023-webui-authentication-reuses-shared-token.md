# 0023. Web UI authentication reuses the existing shared-token middleware

Status: Proposed

## Context

Milestone 4 adds `internal/webui`, a browser-facing HTML frontend that (like
`internal/api`) only calls into `internal/app.Service` for business logic.
`docs/ARCHITECTURE.md` §10 states the intended shape plainly: "Auth already
exists from Milestone 2; this just uses it." The question this ADR settles
is whether "just uses it" means literally reusing `internal/auth`'s existing
`Bearer`/`Basic` middleware unmodified, or whether a browser UI needs its
own authentication layer.

`internal/auth` today is a single shared token, checked two ways
(`middleware.go`): `Bearer` for API clients sending `Authorization: Bearer
<token>`, and `Basic` for anything that only speaks HTTP Basic — which,
against a browser, triggers the browser's native credential popup, since
the username is accepted as any non-empty value and the password is
compared against the one configured token (`ResolveToken`,
`internal/config`, no runtime persistence). There are no users, no roles,
and nothing session-shaped anywhere in this codebase. Once a browser has
supplied correct Basic credentials once, it caches and resends them
automatically for the lifetime of the browser process/profile — there is
no server-side session to invalidate, so there is no way to "log out" short
of the operator closing the browser or rotating the token.

`internal/webui` could instead present its own login page and issue a
session cookie after checking the same shared token once, giving the user
a logout button and a more familiar web-app feel. But that requires a
second credential-checking mechanism running alongside `internal/auth`'s
Bearer/Basic — cookie issuance, session storage or signed tokens,
expiry/rotation, and a logout handler — for a project whose one deliberate
extension mechanism is compile-time registry + blank-import
(`docs/adr/0004-extension-model.md`), and whose stated priority is fewer
moving parts over developer or end-user convenience (`CLAUDE.md`). A
session layer here would be exactly that kind of second, parallel
mechanism, specific to auth.

`docs/adr/0021-transport-security-via-reverse-proxy.md` already establishes
that `receiptd` never terminates TLS itself; the reverse proxy does. In the
one supported deployment model, that means Basic credentials sent by
`internal/webui` never travel in cleartext over the network — the proxy has
already terminated TLS before the request reaches this daemon.

## Decision

`internal/webui` sits behind the same `internal/auth.Bearer`/`Basic`
middleware `internal/api` already uses, checked against the same single
shared token resolved by `internal/auth.ResolveToken`. No new package, no
cookie, no session store, no login page, and no logout handler are added.
A browser hitting an `internal/webui` route with no valid credential gets
the same 401 with `WWW-Authenticate: Basic realm="Receiptd"` that
`internal/api` returns today, which every mainstream browser renders as its
built-in credential prompt — that prompt *is* the Web UI's login screen.
There is no change to `internal/auth`, `internal/config`, or the shape of
the shared token: this is a reuse decision, not a new implementation. This
also complements `docs/adr/0022-webui-server-rendered-html-template.md`'s
plain server-rendered UI: a login page and session cookie would be a
second application model (its own client-side state, its own request flow)
layered on top of the one that ADR already settled on.

## Consequences

- No new package, dependency, or storage is introduced for Web UI auth —
  `internal/webui` adds zero surface area to `internal/auth`, matching the
  "auth already exists, this just uses it" framing in
  `docs/ARCHITECTURE.md` §10.
- **There is no logout button**, and there cannot be one without a session
  to invalidate. The only ways to end a browser's access are closing the
  browser process/profile that cached the credential, or rotating the
  shared token (which also revokes every API client using it).
- The browser caches the Basic credential for the life of the browser
  process/profile, not for a bounded session lifetime — there is no
  expiry, idle timeout, or re-authentication prompt short of that cache
  being cleared.
- On a shared or kiosk-style device (a tablet mounted near the printer,
  for instance), this is a real UX weakness: whoever uses that browser
  next inherits the previous user's cached credential until it's manually
  cleared. This is a genuine cost of the decision, not a hidden one.
- There is no per-user identity anywhere in this model — every request,
  whether from `internal/api` or `internal/webui`, is indistinguishable
  from any other holder of the one shared token. No audit trail can ever
  attribute a print job to a specific person; that would require a users/
  roles model this project has explicitly not built and this ADR does not
  propose.
- Reusing `Bearer`/`Basic` unmodified means `internal/webui` never diverges
  from `docs/adr/0021`'s deployment assumption: this only avoids sending
  credentials in cleartext when the operator runs the supported
  reverse-proxy-terminated-TLS deployment. Exposing `internal/webui`
  directly, with no proxy in front of it, carries the same risk
  `docs/adr/0021` already documents for `internal/api` — this ADR does not
  change that exposure, it inherits it.
- If a future requirement genuinely needs multiple distinguishable users,
  logout, or session expiry, that is a new decision needing its own ADR —
  it is not something to bolt onto `internal/auth` quietly on the
  assumption this ADR was an oversight.

## Alternatives considered

- **Cookie/session layer on top of `internal/auth`**: a login page checks
  the shared token once, then issues a signed or server-stored session
  cookie with its own expiry and a logout handler. Rejected — it is a
  second, parallel credential-checking mechanism specific to auth, adding
  a session store or signing key, expiry/rotation logic, and a login page
  to maintain, for a project that already has exactly one extension
  mechanism and has deliberately not adopted a second anywhere else
  (`docs/adr/0004-extension-model.md`). It would also require deciding
  where a session lives (`internal/config` is immutable after load and
  holds no runtime state, so a session store would be new infrastructure
  with no existing home) for a single-shared-token deployment where no
  session concept exists to protect.
- **`internal/webui`-specific token, separate from the API token**:
  rejected — two tokens for one shared-token security model doubles
  operator configuration (two secrets to provision and rotate) without
  adding a real security boundary, since anyone with either token can
  already reach `internal/app.Service` through whichever surface accepts
  it.
- **Bearer-only for `internal/webui` (no Basic)**: rejected on its own —
  Bearer alone gives a browser nothing to authenticate with automatically;
  without Basic's native prompt, the UI would need its own form to collect
  a bearer token and store it (in a cookie or `localStorage`), which is the
  session-layer alternative above in a thinner disguise, not an avoidance
  of it.
