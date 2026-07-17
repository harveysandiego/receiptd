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

## Conventions

- ADRs are numbered sequentially and never renumbered or deleted.
- If a decision is later reversed or replaced, add a new ADR and mark the
  old one `Superseded by 000X` at the top — don't edit history away.
- Use [template.md](template.md) as the starting point for a new ADR.
- See "How architectural changes should be proposed" in
  [../../CONTRIBUTING.md](../../CONTRIBUTING.md) for the process that
  leads to a new ADR.
