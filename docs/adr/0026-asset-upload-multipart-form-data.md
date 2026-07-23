# 0026. Asset uploads use `multipart/form-data`, not JSON+base64

Status: Proposed

## Context

Milestone 4 (`docs/ARCHITECTURE.md` §10) scopes "image upload, asset
management" as `internal/webui` features, backed by `internal/assets.Store`
(`Put(ctx, name string, data []byte) error` — transport-agnostic; unaffected
by this decision either way). Every existing `internal/api` endpoint
(`preview.go`, `print.go`) is JSON-in/JSON-out: a `receipt.Receipt`'s
`Image` element carries its bytes as base64 inline in the request body,
under the shared `maxRequestBodyBytes` (10 MiB) cap (`internal/api/status.go`).
There is no existing asset-upload handler in either `internal/api` or
`internal/webui` to be consistent with; this ADR settles the encoding for
the new one(s), not a migration of an existing endpoint.

Nothing in `docs/ARCHITECTURE.md` or the package table proposes an
`internal/api` asset endpoint — asset management is listed only under the
Web UI milestone, and `internal/assets/store.go` gestures at "a future
asset-management API handler" without committing it to either package.
`docs/adr/0022-webui-server-rendered-html-template.md` has already settled
that `internal/webui` is plain server-rendered `html/template` with full-page
form submissions or small dependency-free JS — no SPA, no client-side
fetch-based JSON conveniences. An `<input type="file">` inside a form is the
natural way that surface collects a file today.

Two encodings were on the table for the upload handler(s):

1. **JSON + base64** — match `internal/api`'s existing convention: decode a
   JSON body, base64-decode the `data` field, call `Store.Put`.
2. **`multipart/form-data`** — decode via `mime/multipart`, stream the file
   part's bytes, call `Store.Put`.

## Decision

The new asset-upload handler(s) in `internal/webui` accept
`multipart/form-data`, decoded with the standard library's
`net/http`/`mime/multipart` (`r.ParseMultipartForm` /
`r.FormFile`), still bounded by the same `maxRequestBodyBytes` cap applied
via `http.MaxBytesReader` before parsing. The handler reads the uploaded
file's bytes and calls `internal/app.Service`, which in turn calls
`assets.Store.Put(ctx, name, data)` — `Store`'s byte-slice signature needs
no change.

This is scoped to `internal/webui`'s own upload form(s), not to
`internal/api`. `internal/api` keeps its existing JSON-only convention
untouched: this decision adds no multipart handling to that package, and
introduces no second encoding within it. If a future requirement adds a
public, non-browser asset-upload capability to `internal/api`, that is a
separate decision to make on its own terms — not something this ADR
pre-answers by extension. Multipart is introduced solely as the binary
upload transport for this one form; it does not change JSON remaining
this project's default encoding for structured API requests.

## Consequences

- The upload form in `internal/webui` is a plain `<form
  enctype="multipart/form-data">` with a file input — no client-side
  JavaScript needed to read and base64-encode a file, matching
  `docs/adr/0022`'s "no SPA, minimal dependency-free JS" direction.
- Logo images — likely the largest bodies this API ever accepts — travel
  as raw bytes, not inflated ~33% by base64, leaving more of the existing
  10 MiB `maxRequestBodyBytes` cap available as actual file content.
- `internal/webui` now has one handler that parses `multipart/form-data`
  where every other handler in the codebase parses JSON. That is a real,
  named asymmetry — accepted here because it's confined to one package's
  browser-form surface, not a second convention layered onto
  `internal/api`'s existing JSON contract.
- `internal/app.Service`'s method for this still takes a plain `[]byte` (or
  equivalent), so `internal/assets.Store` and `app.Service`'s contract stay
  identical regardless of which package or encoding calls them — this
  decision is fully contained in the HTTP-decoding layer of one handler.
- Should `internal/api` ever need its own asset-upload capability, it
  would reasonably use JSON+base64 to stay consistent with itself, meaning
  the codebase would then genuinely have two encodings for "upload an
  asset" — a cost to weigh explicitly in that future ADR, not one this
  decision resolves in advance.

## Alternatives considered

- **JSON + base64, matching `internal/api`'s existing convention.**
  Rejected for this handler: there is no existing `internal/api` asset
  endpoint to be consistent with, the upload only needs to exist behind
  `internal/webui`'s form, and paying a ~33% size inflation plus requiring
  hand-written client-side base64-encoding JavaScript contradicts
  `docs/adr/0022`'s plain-form direction for no offsetting benefit.
- **Add the asset-upload handler to `internal/api` instead, using JSON.**
  Rejected — nothing in `docs/ARCHITECTURE.md` scopes asset management as
  an `internal/api` capability; it's listed only under the Web UI
  milestone. Building a public REST capability that isn't asked for is
  speculative scope this project's philosophy (`CLAUDE.md`) rejects by
  default.
- **`multipart/form-data` handled via a third-party form/upload library.**
  Rejected — `mime/multipart` and `http.MaxBytesReader` are both standard
  library and already used elsewhere in this codebase for the size cap;
  there is no second concrete need to justify a dependency here.
