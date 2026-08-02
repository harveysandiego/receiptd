# 0029. The asset content endpoint serves an inline-type allowlist, everything else downloads

Status: Accepted

## Context

The Web UI's Assets page needs to show what a stored asset actually *is* —
a thumbnail per row, clickable through to the full-size image. That
requires a route serving an asset's bytes back to the browser:
`GET /assets/{name}/content`.

This is the first place Receiptd returns caller-supplied bytes to a
browser as a document rather than as data inside a JSON field. That makes
it a stored-XSS surface, and the details matter:

- The uploader chooses **both** the asset name and its bytes
  (`docs/adr/0026`'s multipart form). Neither is trustworthy on its own.
- The Web UI sits behind shared-token auth (`docs/adr/0023`) with no
  session cookie, but a script executing on this origin can still read and
  act on every page the operator's browser can reach, including the print
  and asset-management forms.
- `internal/webui`'s `securityHeaders` already sets
  `X-Content-Type-Options: nosniff` on every response
  (`internal/webui/router.go`), which stops a browser second-guessing a
  `Content-Type` we send — but it does nothing about a `Content-Type` that
  is *correctly* dangerous, which is exactly the SVG case.
- `assets.Store` records no MIME metadata; it stores names and bytes. The
  type has to be determined at serve time.

## Decision

`GET /assets/{name}/content` serves an asset inline only when **both** of
these agree:

1. Its extension is in a closed allowlist — `.png`, `.jpg`, `.jpeg`,
   `.gif`, `.webp`.
2. `http.DetectContentType` on the asset's own bytes reports that same
   media type.

When they agree, the response carries that image `Content-Type`. Every
other asset — a mismatch, an unlisted extension, no extension at all —
is served as `application/octet-stream` with a `Content-Disposition:
attachment` header, so the browser downloads it instead of rendering it.

**SVG is deliberately excluded** from the allowlist. It is an image by
every ordinary definition, and a reasonable format for a logo, but a
browser rendering an SVG document executes any `<script>` inside it. There
is no `Content-Type` at which an attacker-supplied SVG is safe to serve
inline from the same origin as the admin UI.

The endpoint sets `Cache-Control: no-store`, because `Store.Put`
overwrites in place and a cached response would show the previous asset
under an unchanged name.

The listing page decides between an `<img>` and a download link using the
extension alone, since it has no bytes in hand. That is a display hint
only — the handler's two-part check is what actually gates serving, and it
runs on every request regardless of what the page rendered.

## Consequences

- Thumbnails work for the formats a thermal-printer logo is realistically
  stored in, which is the case this feature exists to serve.
- An SVG logo uploads, prints, and downloads fine, but shows a download
  link rather than a thumbnail. This is a real, visible feature gap
  accepted on purpose. `receipt.Asset` resolution is unaffected — that
  path never goes through this endpoint.
- The cross-check rejects a validly-named image whose bytes are corrupt or
  truncated below the sniff window. It downloads instead of rendering,
  which is the right failure direction but may briefly confuse someone who
  uploaded a broken file.
- The allowlist is a maintenance point: a genuinely new raster format
  (AVIF, say) needs a deliberate edit here plus a check that
  `http.DetectContentType` recognises it. Preferred over an open policy
  that silently admits whatever the standard library learns to sniff.
- Adding a *second* place that serves stored bytes to a browser would need
  to apply the same rule; it is one function (`internal/webui/assetmime.go`)
  rather than logic inlined in a handler, so reusing it is the easy path.

## Alternatives considered

- **Trust `mime.TypeByExtension` on the name alone.** Rejected. The
  uploader picks the name, so this serves attacker-chosen bytes under an
  attacker-chosen type. `nosniff` would stop the browser from *correcting*
  the type, but that is the wrong lever: it enforces our claim, and our
  claim would be the attacker's.
- **Trust `http.DetectContentType` on the bytes alone.** Rejected, though
  much closer to safe. It ignores the name entirely, so a `.svg` whose
  bytes sniff as `text/xml` or `text/plain` still ends up served under a
  type some browsers will render. Requiring both narrows it to types we
  have actually enumerated.
- **Serve SVG inline with a restrictive per-response CSP
  (`sandbox`/`script-src 'none'`).** Rejected for now. It is a legitimate
  technique, but it makes the safety of the whole endpoint depend on
  getting a second, per-response CSP exactly right and on every supported
  browser honouring it — a lot of load-bearing complexity for one image
  format, in a UI where the download link is a perfectly good fallback.
  Worth revisiting only if SVG logos turn out to matter in practice.
- **Serve assets from a separate origin or subdomain.** The standard
  robust answer to user-uploaded content, and genuinely the strongest.
  Rejected as wrong for this project: Receiptd is a single self-hosted
  binary on a home network, frequently at a bare IP, with TLS delegated
  entirely to a reverse proxy (`docs/adr/0021`). Requiring a second
  hostname to view a logo thumbnail would impose real deployment cost on
  every user to close a hole the allowlist already closes.
