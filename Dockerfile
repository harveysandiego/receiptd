# syntax=docker/dockerfile:1

# --- build stage ---------------------------------------------------------
# golang:1.25 matches go.mod's `go 1.25.0` minimum. Pinned to a specific
# minor version (not "latest") for reproducible builds — bump this
# alongside go.mod's `go` directive, not independently of it.
FROM golang:1.25-bookworm AS build

WORKDIR /src

# Module download is its own layer, cached independently of source
# changes, so editing internal/ or cmd/ doesn't force a re-download. The
# cache mount persists /go/pkg/mod across separate `docker build`
# invocations too (not just across layers within one build), so a
# second local or CI build doesn't re-download unchanged modules at all.
# This only speeds up builds — BuildKit excludes cache mounts from the
# resulting image, so it has no effect on image content.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# TARGETOS/TARGETARCH are populated by buildx for whichever platform is
# currently being built, so `docker buildx build --platform
# linux/amd64,linux/arm64 .` cross-compiles both from this one stage
# without any per-arch branching in this file.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# CGO_ENABLED=0 produces a fully static binary with no libc dependency —
# see README's "small, static binary" design goal — which is what makes
# the distroless "static" runtime base below possible at all. The
# go-build cache mount persists compiled package objects across builds
# the same way the module cache above does — same caveat: build speed
# only, no effect on the resulting binary.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/receiptd \
    ./cmd/receiptd

# The runtime base below ships no shell and no coreutils, so the
# directories receiptd needs at runtime (its config location and its
# writable data root) have to be created and given the runtime user's
# ownership here, then copied over as-is.
RUN mkdir -p /out/rootfs/etc/receiptd \
             /out/rootfs/var/lib/receiptd/assets && \
    chown -R 65532:65532 /out/rootfs

# --- runtime stage ---------------------------------------------------------
# distroless "static" + "nonroot": no shell, no package manager, not even
# libc — the smallest attack surface available for a CGO_ENABLED=0
# static binary, and it already runs as a non-root user (uid/gid 65532)
# out of the box, which matters here since this base has no `useradd` to
# create one with anyway.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build --chown=65532:65532 /out/receiptd /receiptd
COPY --from=build /out/rootfs/etc/receiptd /etc/receiptd
COPY --from=build /out/rootfs/var/lib/receiptd /var/lib/receiptd

USER nonroot:nonroot

# Advisory only (EXPOSE doesn't publish anything by itself) — matches
# the server.address a container config should bind, e.g. "0.0.0.0:8080",
# documented in README's Docker section.
EXPOSE 8080

ENTRYPOINT ["/receiptd"]
CMD ["--config", "/etc/receiptd/config.yaml"]
