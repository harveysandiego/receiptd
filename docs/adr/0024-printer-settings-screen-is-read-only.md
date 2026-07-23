# 0024. The Web UI's printer settings screen is read-only

Status: Proposed

## Context

`docs/ARCHITECTURE.md` §10 lists "printer settings" among Milestone 4's
`internal/webui` feature set, alongside printer status, quick actions,
preview, text printing, image upload, and asset management. Before that
screen is built, one question needs a recorded answer: does it just
*display* each configured printer's name, transport, address, profile,
and current `printer.Printer.Status`, or can an operator *edit* those
values from the browser and have the change take effect?

`internal/config` loads `printers[]` from YAML exactly once, in
`config.Load` at daemon startup, via `os.ReadFile` followed by
`Config.Validate()`. There is no code path anywhere in this codebase —
in `config`, `app`, `api`, or elsewhere — that writes a `Config` (or any
part of it) back to disk, or otherwise persists a runtime change to it.
`Config` is, in practice, immutable for the life of the process. Making
the printer settings screen editable would require inventing that
capability from scratch: a serializer back to YAML (or a second config
store entirely), file-locking or atomic-write handling for a
`config.yaml` an operator might also be hand-editing, a decision about
whether an in-place edit applies live or requires a restart, and a
validation path that reuses `Config.Validate()` without re-running the
whole `config.Load` startup sequence.

The deployment target set by `CLAUDE.md` and reaffirmed in
`docs/adr/0021-transport-security-via-reverse-proxy.md` is a homelab or
Raspberry Pi host where an operator who can SSH in, edit `config.yaml`,
and restart the systemd unit is the assumed norm, not a burden being
routed around. Auth is a single shared Bearer/Basic token
(`internal/auth`) with no user/role model — there is no notion of "which
operator is allowed to repoint this printer at a different IP" for an
editable screen to enforce beyond the one token everyone already shares.

## Decision

The printer settings screen in `internal/webui` is **read-only**. It
renders each printer's name, transport, address, profile (width, DPI,
cut support), and current `printer.Printer.Status` (sourced through
`internal/app.Service`, the same entry point `internal/api` already
uses), straight out of the `internal/config` values loaded at startup.
There is no form, no submit action, and no request path — in `webui` or
`api` — that accepts a change to any of these fields.

**Runtime editing of printer configuration is explicitly out of scope
for this decision.** It is not deferred implicitly or left as an
unstated future step: if it is ever pursued, it needs its own ADR that
makes the case for adding a config-persistence capability this codebase
does not have today, and that reasons through the questions above
(storage format, concurrent-edit safety against a hand-edited
`config.yaml`, live-apply vs. restart-required, re-validation). This ADR
does not pre-approve that work by leaving the door open.

## Consequences

- No in-browser way to add a printer, fix a typo'd IP address, or swap a
  printer's `model:` for a `profile:` block — every such change still
  requires editing `config.yaml` and restarting the `receiptd` unit. On
  the stated deployment target, that's an SSH session and a `systemctl
  restart`, not a blocked operator.
- `internal/config` gains no writer, no serialization-back-to-YAML path,
  and no on-disk locking story — the "load once, validate, done"
  contract documented in `docs/ARCHITECTURE.md` §7 stays exactly true
  after Milestone 4 lands, not true "until the printer settings screen."
- The screen needs no new `apperr.Kind` for a rejected edit, no new
  authorization question beyond the existing shared-token check, and no
  new `app.Service` method beyond reading printer configuration and
  `Status` — it is a strict subset of what `internal/api` already
  exposes for printer listing/status.
- If a future ADR does add runtime config persistence, this ADR's
  Context section is the honest record of what that would have to solve
  that doesn't exist yet — it isn't a small addition to bolt on.

## Alternatives considered

- **Editable settings screen, writing straight back to `config.yaml`**:
  rejected — introduces a config-writer with no story for concurrent
  edits against an operator hand-editing the same file over SSH, no
  answer for atomic writes, and no answer for whether a change applies
  live or needs a restart anyway (in which case the "convenience" is
  mostly illusory). This is real, non-speculative infrastructure this
  project doesn't have today, being proposed as a side effect of a UI
  screen rather than justified on its own.
- **Editable settings, persisted to a separate runtime store (e.g. a
  small bbolt bucket) that overlays the YAML at read time**: rejected —
  a second source of truth for the same configuration values, split
  across two files an operator has to know to check, for a homelab
  deployment where editing one YAML file was never the actual obstacle.
  Also a second config-loading mechanism alongside the one this project
  already has, contrary to "minimal packages, no speculative
  abstractions."
- **Editable but gated behind a confirmation that only takes effect
  after a manual restart** (a "pending change" queued in memory):
  rejected as a half-measure — it still needs a persistence path to
  survive the very restart it requires, so it carries all the cost of
  the first alternative while delivering less: an operator still has to
  touch the machine, just through a form instead of a text editor.
