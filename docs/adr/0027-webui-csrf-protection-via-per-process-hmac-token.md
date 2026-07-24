# 0027. CSRF protection via a per-process HMAC token, not a session cookie

Status: Accepted

## Context

`docs/adr/0023-webui-authentication-reuses-shared-token.md` put `internal/webui`
behind the same `auth.Basic` middleware `internal/api` uses, checked against
one shared token. That decision's own Consequences section already names
the relevant fact: "once a browser has supplied correct Basic credentials
once, it caches and resends them automatically for the lifetime of the
browser process/profile." A browser does this **per origin**, automatically,
for every subsequent request — including one triggered by a completely
different site the same browser happens to have open in another tab.

That is a textbook CSRF setup: a malicious page (anywhere) can render an
auto-submitting form targeting `POST /print`, `POST /assets`, or `POST
/assets/{name}/delete` on `receiptd`'s address, and the victim's browser
will attach the cached Basic credential to that cross-origin request
without the malicious page ever needing to know the token itself. Every
state-changing `internal/webui` route is exposed to this today.

The standard defense — a synchronizer token embedded in the form and
checked against a value stored server-side per session — doesn't have an
obvious home here. `docs/adr/0023` was explicit and deliberate: "No new
package, no cookie, no session store, no login page, and no logout
handler are added," specifically to avoid a second, parallel
credential-adjacent mechanism next to the one shared token this project
already has. Reaching for a session cookie now, purely to hold a CSRF
token, would reopen exactly the question that ADR settled — session
storage, expiry, and a home for that state in a codebase where
`internal/config` is immutable after load and holds no runtime state
(`docs/adr/0023`'s own Alternatives section made the same point about a
login-session cookie).

## Decision

`internal/webui` protects every state-changing route (`POST /print`,
`POST /assets`, `POST /assets/{name}/delete`) with a synchronizer-style
CSRF token that needs **no session, no cookie, and no per-user state**:

- At process start, `internal/webui` generates one random 32-byte secret
  via `crypto/rand`, kept only in memory (`csrf.go`). It is never logged,
  never persisted, and never sent to a client in any form.
- The token embedded in every protected form's hidden `csrf_token` field
  is `base64(HMAC-SHA256(secret, fixedMessage))` — the same value for the
  life of the process, not freshly randomized per render or per request.
- Every protected `POST` handler calls `r.ParseForm()` (or
  `ParseMultipartForm`, for the multipart upload) and then verifies the
  submitted `csrf_token` against the current token via
  `crypto/subtle.ConstantTimeCompare`, before doing anything else. A
  missing or mismatched token is an ordinary `apperr.KindValidation`
  failure, rendered through the same `renderError` path any other bad
  request uses — no new error surface.
- `POST /preview` is **not** protected: `Service.Preview` never mutates
  state (docs/adr/0006), so it sits outside this decision's scope, the
  same way it already sits outside the PRG-redirect pattern documented in
  `docs/ARCHITECTURE.md`'s "Milestone 4 as built" section.

This works without a session because the property CSRF protection
actually needs is "the attacker's page cannot produce a valid token,"
not "the server remembers who has one." The Same-Origin Policy already
guarantees a cross-origin page cannot read the hidden field out of a
`receiptd`-rendered page, so it can never learn the token value — the
`secret` never needing to leave server memory is what makes the token
unforgeable, not any concept of a session.

## Consequences

- No new package, cookie, or session store — `internal/webui`'s import
  graph is unchanged, and `docs/adr/0023`'s "no session anywhere in this
  model" stays true.
- The token is static for a process's lifetime: restarting `receiptd`
  invalidates every previously-rendered form still open in a browser tab
  (the next submission fails CSRF validation and the operator just
  reloads the page) — a minor, occasional inconvenience, not a security
  gap.
- Because the token never rotates within a process's lifetime, a token
  that somehow leaked (e.g. a misconfigured proxy logging full request
  bodies) would stay valid until the next restart. Given this project's
  homelab/single-operator deployment target and that transport is
  expected to run behind TLS (`docs/adr/0021`), this is judged an
  acceptable trade for avoiding a session store — revisit if a future
  deployment model needs a stronger guarantee.
- `POST /preview` stays intentionally unprotected; if `Service.Preview`
  ever gains a side effect, that would be the point to reconsider this
  scope, not something this ADR pre-decides.
- Every protected handler now calls `r.ParseForm()`/`ParseMultipartForm`
  before doing anything else, including `POST /assets/{name}/delete`,
  which previously read only the URL path — a small, mechanical change
  to how that one handler reads its request, not a behavior change to
  what it does once verified.

## Alternatives considered

- **Session cookie + synchronizer token**: rejected — reopens the
  session-store question `docs/adr/0023` already closed, for a project
  whose one extension/state mechanism deliberately excludes this shape
  of new infrastructure.
- **Double-submit cookie** (a CSRF cookie set by the server, compared
  against a form field): rejected as no simpler than the chosen
  approach, while still introducing `internal/webui`'s first cookie —
  the HMAC-token approach gets the same unforgeability property without
  adding any cookie at all.
- **Origin/Referer header validation only** (no token): a legitimate,
  lighter-weight defense-in-depth technique, but weaker on its own — it
  depends on browsers reliably sending `Origin`/`Referer` and on every
  intermediary preserving them, and gives no server-side proof the
  request came from a page that actually loaded the form. Rejected as
  the sole mechanism; nothing here precludes adding it later as a second,
  independent layer if a real gap is found.
- **Third-party CSRF middleware**: rejected — the whole mechanism is
  under 40 lines of standard-library `crypto/hmac`/`crypto/subtle`, well
  within "minimal packages, no speculative dependencies" (`CLAUDE.md`),
  and there was no second concrete need to justify the dependency.
