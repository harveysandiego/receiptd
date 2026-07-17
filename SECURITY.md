# Security Policy

<!-- URLs below assume github.com/harveysandiego/receiptd — update if forked/renamed. -->

## Supported versions

Receiptd is pre-1.0. Until a 1.0 release, only the latest tagged release
receives security fixes. Once versioning stabilizes, this table will list
supported major versions explicitly.

| Version        | Supported          |
|----------------|---------------------|
| latest release | :white_check_mark:  |
| older releases | :x:                 |

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, use GitHub's private vulnerability reporting:

1. Go to the [Security tab](https://github.com/harveysandiego/receiptd/security) of this repository.
2. Click "Report a vulnerability".
3. Include as much detail as you can: affected version/commit, reproduction
   steps, and potential impact.

You should expect an initial response within 7 days. If the report is
confirmed, we will work with you on a fix and coordinated disclosure
timeline before any public details are published.

## Scope

Receiptd exposes a REST API and Web UI intended to run on a trusted local
network (home lab, homeserver, Raspberry Pi). Relevant security concerns
include, but are not limited to:

- Authentication/authorization bypass in `auth` (bearer token / basic auth)
- Path traversal or injection via the `assets` store or uploaded images
- Denial of service via malformed Receipt JSON (e.g. pathological
  `columns`/`table` nesting) that causes excessive CPU/memory in `render/layout`
  or `render/canvas`
- Any issue that would let a request reach the physical printer or job
  queue without passing through authentication when `auth.enabled: true`

Receiptd is explicitly **not** designed to be exposed directly to the
public internet without a reverse proxy and your own additional hardening;
this is documented in the README under Installation.

## Disclosure

Once a fix is released, a GitHub Security Advisory will be published
against this repository, crediting the reporter unless they request
otherwise.
