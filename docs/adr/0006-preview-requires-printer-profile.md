# 0006. Preview requires an explicit printer target

Status: Accepted

## Context

`app.Service.Preview`'s signature was frozen in the original architecture
review as `Preview(ctx context.Context, r receipt.Receipt) ([]byte, error)`
— no printer name, unlike `Print`, which already resolves a
`printer.Profile` from a `printerName` string. This was consistent at the
time: through Milestone 2, `render/canvas.Paint` sized the `Canvas` purely
from painted content (the widest line's measured width), so no stage of
`Preview`'s rendering path ever consulted a `printer.Profile` at all.

This became a genuine conflict while implementing the "thread
`printer.Profile` through layout and rendering" slice (Milestone 3):
`render/layout.Build` and `render/canvas.Paint` need to size the `Canvas`
to `profile.WidthDots` instead of to content, so the rendering pipeline
they're both part of — shared by `Process` and `Preview` via
`app.Service.render` (see `internal/app/process.go`,
`internal/app/preview.go`) — now needs a `printer.Profile` to run at all.
`Process` already has one, resolved from `Job.PrinterName`. `Preview` has
never had a printer name anywhere in its signature or in the
`POST /api/v1/preview` wire format, and nothing in this document specifies
a "default printer" concept to fall back on.

This is concrete evidence discovered while implementing the slice, not a
hypothetical: without resolving it, the profile-threading slice cannot be
completed without either inventing unwritten policy for `Preview`, or
silently reintroducing two divergent rendering paths — both of which
`docs/adr/0001-receipt-model.md` already rejected ("There is exactly one
rendering pipeline... no input source gets its own parallel path").

## Decision

`Preview` takes an explicit `printerName string`, exactly mirroring
`Print`:

```go
func (s *Service) Preview(ctx context.Context, r receipt.Receipt, printerName string) ([]byte, error)
```

The `POST /api/v1/preview` request body gains a `printer` field, mirroring
`POST /api/v1/print`'s existing `printRequest{ Printer, Receipt }` shape:

```json
{ "printer": "front-desk", "receipt": { "version": 1, "elements": [...] } }
```

An unknown or missing `printerName` is `apperr.KindNotFound` — the same
behavior `Process` already has for an unconfigured `Job.PrinterName` — not
a silently-picked default.

This keeps `app.Service.render` (the function `Preview` and `Process`
already share) as the one rendering path for both, now uniformly
`printer.Profile`-aware. Callers of `Preview` — the raw
`/api/v1/preview` endpoint, the `POST /api/v1/templates/weather/preview`
convenience endpoint (`docs/ARCHITECTURE.md` §4 step 6), and the CLI's
`preview` command — all now need to identify a target printer up front,
the same way they already do (or, for the template endpoints, will) for
printing.

## Consequences

- Exactly one rendering path continues to serve both preview and print —
  no default-profile invention, no non-deterministic behavior, and a
  preview now accurately reflects the real target printer's width rather
  than an arbitrary content-fit guess.
- This is a breaking change to `Preview`'s Go signature and the
  `/api/v1/preview` wire format: any existing caller supplying a bare
  `Receipt` with no `printer` field will need to add one.
  Receiptd is pre-alpha with no release yet (`README.md` "Current
  status"), so this is an acceptable time to make the change — there is no
  installed base to migrate.
- The CLI's `preview` command gains a `--printer` flag, matching `print`'s
  existing one — no new concept for a CLI user, since they already have to
  know a printer name to actually print.
- A user who wants to preview a receipt without committing to a specific
  target printer (e.g. "what would this look like on any 80mm printer")
  has no way to do that anymore. Not a goal this design ever supported —
  `printer.Profile` is the only source of paper width in this schema (see
  `docs/adr/0001-receipt-model.md`) — but worth naming as a capability
  this decision forecloses rather than one it neutrally leaves alone.

## Alternatives considered

- **Implicit default / first-configured printer**: rejected. Go map
  iteration order (`app.Service.Profiles`) is not deterministic, which
  would directly violate the rendering-determinism requirement this same
  slice is implementing; there is also no config field anywhere that
  designates a "default" printer to fall back to.
- **Fixed, hardcoded fallback `Profile`** (e.g. a nominal 80mm/203dpi
  profile used only for preview sizing): rejected. It invents a number
  with no basis in any configured printer, and a preview could visually
  disagree with the real printer's actual width — actively misleading
  rather than merely incomplete.
- **Two rendering paths** (profile-aware for `Process`, content-fit for
  `Preview`): rejected. Reintroduces the exact "parallel code path per
  input source" duplication `docs/adr/0001-receipt-model.md` already
  rejected for a different axis (input format) — the same reasoning
  applies here across output purpose (preview vs. print).
