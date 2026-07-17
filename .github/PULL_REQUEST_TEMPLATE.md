## Summary

<!-- What does this change do, and why? Link related issues with "Closes #123". -->

## Type of change

- [ ] Bug fix
- [ ] New feature (Element type, template, provider, endpoint)
- [ ] Refactor (no behavior change)
- [ ] Documentation
- [ ] CI / tooling
- [ ] Architectural change (see below)

## Architectural impact

- [ ] This PR does **not** change any interface, package boundary, or
      concept documented in `docs/ARCHITECTURE.md`.
- [ ] This PR **does** change something in `docs/ARCHITECTURE.md` — I have
      updated that document and, if the change is significant, added or
      amended an ADR in `docs/adr/`, per `CONTRIBUTING.md`.

## Checklist (Definition of Done — see CONTRIBUTING.md)

- [ ] Tests added/updated for the change (unit tests at minimum; golden
      tests for `render/canvas` / `render/escpos` changes)
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run ./...` passes
- [ ] `gofmt` / `goimports` clean
- [ ] Documentation updated alongside the code (godoc comments, README,
      ARCHITECTURE.md, or an ADR, as applicable)
- [ ] Commit messages follow the convention in `CONTRIBUTING.md`

## How was this tested?

<!-- Describe what you ran: unit tests, manual CLI/API calls, real hardware, etc. -->

## Screenshots / sample output

<!-- If this changes rendering output, a receipt preview PNG is very helpful. -->
