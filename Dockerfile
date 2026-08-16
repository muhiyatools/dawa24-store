# Multi-stage build, mirroring the pattern already proven in the MuhiyaLLM
# Gateway image so both services behave the same way in the Elest.io pipelines.

FROM golang:1.26-bookworm AS builder

WORKDIR /app

# Dependencies first so the module cache layer survives source edits.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

# CGO_ENABLED=0 produces a static binary that runs on a slim base image.
# -trimpath keeps build paths out of the binary.
ENV CGO_ENABLED=0
RUN go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT}" \
      -o /out/server ./cmd/server && \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    go build -trimpath -ldflags="-s -w" -o /out/cli    ./cmd/cli

FROM debian:bookworm-slim

WORKDIR /app

# ca-certificates for outbound TLS to the Gateway and object storage.
# curl for the container healthcheck.
# tzdata because the platform operates in Africa/Cairo and timestamps are
# rendered in local time even though they are stored as UTC.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl tzdata && \
    rm -rf /var/lib/apt/lists/*

# A container escape in a root-owned process has host-level blast radius it does
# not need. Same reasoning as the Gateway image.
RUN groupadd -r dawa24 && useradd -r -g dawa24 -d /app dawa24

COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/cli    /app/cli
COPY --from=builder /app/internal/ui/static /app/internal/ui/static

RUN chown -R dawa24:dawa24 /app
USER dawa24

EXPOSE 8080

# Liveness only. Readiness (which includes migrations and dependencies) is
# checked by the platform against /ready.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD curl -fsS http://localhost:8080/health || exit 1

CMD ["/app/server"]
