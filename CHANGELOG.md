# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/) —
see [VERSIONING.md](VERSIONING.md) for what that means in practice during
the 0.x series.

## [Unreleased]

### Added

- Repository scaffolding: architecture documentation, ADRs, CI/CD,
  contribution guidelines, and the initial Go package skeleton. No
  Receiptd functionality yet — see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
  for the roadmap.
- Multi-stage `Dockerfile` and `.dockerignore` for local container builds:
  a `CGO_ENABLED=0` static build layered onto a distroless, non-root
  runtime image. No application code changes — see the Docker section of
  [README.md](README.md#docker) for build/run instructions.

## [0.1.0] - (planned, not yet released)

Placeholder for the first tagged release, corresponding to
[Milestone 1](docs/ARCHITECTURE.md#10-roadmap) (local render, no server):
`receipt` model + `Validate()`, `apperr`, `render/layout`, `render/canvas`,
and the offline `receipt render receipt.json --out preview.png` CLI path.
This entry will gain a real date and a filled-in list of changes — moved
out of Unreleased above — once that milestone actually ships.

[Unreleased]: https://github.com/harveysandiego/receiptd/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/harveysandiego/receiptd/releases/tag/v0.1.0
