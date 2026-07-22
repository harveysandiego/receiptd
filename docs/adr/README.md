# Architecture Decision Records

This directory records the significant architectural decisions behind
Receiptd — the *why*, not the *what* (the current design is described in
[docs/ARCHITECTURE.md](../ARCHITECTURE.md)). The goal is that a future
contributor — human or AI — can understand why something looks the way it
does without reopening a discussion that already happened.

## Index

| ADR | Title |
|---|---|
| [0001](0001-receipt-model.md) | Receipt as a printer-agnostic document model |
| [0002](0002-raster-rendering.md) | Raster-first rendering over native ESC/POS commands |
| [0003](0003-print-queue.md) | Asynchronous, persistent print queue from day one |
| [0004](0004-extension-model.md) | Compile-time registration over runtime plugins |
| [0005](0005-error-handling.md) | Typed errors via a Kind + Op + wrapped-cause taxonomy |
| [0006](0006-preview-requires-printer-profile.md) | Preview requires an explicit printer target |
| [0007](0007-bitmap-text-styling.md) | Integer bitmap scaling as the public text-styling API |
| [0008](0008-embedded-font-legibility.md) | Doubling the embedded font's native resolution for legibility |
| [0009](0009-barcode-symbologies.md) | Fixed set of barcode symbologies for v1 |
| [0010](0010-printer-control-elements-via-canvas-controls.md) | Positioned printer-control elements carried via Canvas.Controls |
| [0011](0011-divider-thickness-legibility.md) | Raising the default divider thickness for hardware legibility — superseded by 0012 |
| [0012](0012-divider-thickness-default-and-scaling.md) | Lowering the default divider thickness and adding a Size scale factor |
| [0013](0013-text-and-asset-alignment.md) | Closing the Text.Align/Asset.Align/Asset.Width gap with pixel- and space-padding |
| [0014](0014-list-elements.md) | Lists: a single `list` element for bulleted, numbered, and checkbox items |
| [0015](0015-printer-model-catalogue.md) | A known-model printer catalogue, not a paper-width heuristic |
| [0016](0016-queue-concurrency-per-printer-workers.md) | Per-printer worker concurrency and atomic Job claiming |
| [0017](0017-queue-lifecycle-crash-recovery.md) | Startup reconciliation for Jobs orphaned by a daemon crash while Running |
| [0018](0018-graceful-shutdown.md) | Bounded graceful shutdown on SIGTERM/SIGINT, without interrupting an in-flight print |
| [0019](0019-retry-pipeline-granularity.md) | Retry the whole render→encode→send pipeline, not just Send |
| [0020](0020-idempotent-print-requests.md) | Idempotent print requests via a client-supplied key, deduped at enqueue time |

## Conventions

- ADRs are numbered sequentially and never renumbered or deleted.
- If a decision is later reversed or replaced, add a new ADR and mark the
  old one `Superseded by 000X` at the top — don't edit history away.
- Use [template.md](template.md) as the starting point for a new ADR.
- See "How architectural changes should be proposed" in
  [../../CONTRIBUTING.md](../../CONTRIBUTING.md) for the process that
  leads to a new ADR.
