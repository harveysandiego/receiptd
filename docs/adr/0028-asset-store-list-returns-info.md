# 0028. `assets.Store.List` returns `[]Info`, not `[]string`

Status: Accepted

## Context

The Web UI's Assets page (`internal/webui/assets.go`, Milestone 4) lists
stored assets by name and nothing else, because that is all
`assets.Store.List` reports:

```go
List(ctx context.Context) ([]string, error)
```

Adding an asset browser — thumbnails plus a size and modified time per row
— needs two facts `List` doesn't carry. Both implementations already have
them at the moment `List` runs: `filesystemStore` calls `os.ReadDir`, whose
`fs.DirEntry` exposes `Info()` with size and mod time; on Windows this is
free (`FindNextFile` already returned the metadata), while on Unix it costs
one additional `lstat` per entry — still one `lstat` per asset rather than
the `Stat`-per-row the rejected alternative below requires. `memoryStore`
knows `len(data)` outright.

`app.AssetSummary`'s own doc comment already anticipated this ("a later
field (size, content type) can be added without changing
`Service.ListAssets`'s return type"), so the question was never *whether*
to expose the metadata, only where it enters the system.

`Store` is a documented interface (`docs/ARCHITECTURE.md` §2), so changing
it is not a local decision. Two shapes were considered, below.

## Decision

`List` returns a named struct type rather than bare names:

```go
type Info struct {
    Name    string
    Size    int64
    ModTime time.Time
}

List(ctx context.Context) ([]Info, error)
```

`Get`, `Put`, and `Delete` are unchanged. In particular `Get` — the only
method `render/layout.Build` calls, to resolve a `receipt.Asset` — keeps
its exact signature, so nothing downstream of the Receipt is affected by
this change.

`memoryStore` tracks a `modTime` per entry, set on `Put`, purely so both
implementations report the same fields. `store.go` already commits them to
identical observable behaviour ("a caller can't depend on behaviour that
happens to differ between them"); a zero `ModTime` from one but not the
other would break exactly that promise.

`Info` deliberately carries no content type. Which types are safe to
render inline in a browser is a presentation and security concern, decided
in `docs/adr/0029-asset-content-endpoint-inline-type-allowlist.md` and
implemented in `internal/webui` — not a property of a stored byte slice.

## Consequences

- The Assets page gets size and modified time in the same call it already
  made, with no second round-trip per row (just, on Unix, the per-entry
  `lstat` `Info()` already does).
- `List` is a wider contract than it was: both implementations must now
  populate three fields correctly, and `memoryStore` carries a `modTime`
  it has no intrinsic need for. That is a real cost, paid to keep the two
  implementations behaviourally identical.
- A breaking change to a documented interface. It is contained — `List`
  had exactly one production caller (`app.Service.ListAssets`) — but it is
  still a change any out-of-tree implementation of `assets.Store` would
  have to follow. Nothing out of tree exists today.
- `Info` is a natural place for a later field (an ETag, a checksum) to
  land without changing the signature again, the same way `AssetSummary`
  absorbed this change without changing `ListAssets`'s.

## Alternatives considered

- **Add `Stat(ctx, name) (Info, error)` and leave `List` alone.**
  Rejected. It is additive rather than breaking, which is its one genuine
  advantage, but the Assets page would then call `List` followed by one
  `Stat` per row — N extra syscalls for data `os.ReadDir` had already
  returned and thrown away. It also creates a pairing every caller has to
  remember, and two ways to ask about one asset.
- **Compute the size in `app.ListAssets` by calling `Get` per asset.**
  Rejected outright: it reads every asset's full bytes into memory to
  display a size column, and still has no answer for modified time.
- **Return `[]fs.FileInfo` or `[]fs.DirEntry`.** Rejected — both are
  filesystem-shaped types leaking into an interface whose whole point is
  that storage might not be a filesystem. `memoryStore` would have to
  fabricate a fake `FileInfo` to satisfy it.
