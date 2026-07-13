# ── Stage 1: Build ────────────────────────────────────────────────
FROM golang:1.26-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64

ENV CGO_ENABLED=0
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

RUN go build \
    -ldflags="-s -w" \
    -o /lunar-ics .

# ── Stage 2: Debug (distroless, rootless) ────────────────────────
ARG SOURCE_URL=https://github.com/lunar-ops/lunar-ics

FROM gcr.io/distroless/static-debian12 AS debug

LABEL org.opencontainers.image.title="lunar-ics-debug" \
      org.opencontainers.image.description="Debug image for lunar-ics (non-root, distroless)" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      debug=true

COPY --from=builder /lunar-ics /lunar-ics

# Run as non-root (nobody:65534) — compatible with readOnlyRootFilesystem
USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/lunar-ics"]

# ── Stage 3: Release (distroless, rootless) ───────────────────────
ARG SOURCE_URL=https://github.com/lunar-ops/lunar-ics

FROM gcr.io/distroless/static-debian12 AS release

LABEL org.opencontainers.image.title="lunar-ics" \
      org.opencontainers.image.description="Rootless, distroless, stateless image for lunar-ics. Runs with readOnlyRootFilesystem." \
      org.opencontainers.image.source="${SOURCE_URL}"

# CA certificates are included in distroless/static-debian12 for HTTPS support
COPY --from=builder /lunar-ics /lunar-ics

# Run as non-root (nobody:65534) — compatible with readOnlyRootFilesystem
USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/lunar-ics"]
