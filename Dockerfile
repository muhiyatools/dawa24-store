FROM golang:bookworm AS builder

WORKDIR /app

# 1. Cache dependencies layer (only re-downloads when go.mod or go.sum change)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 2. Copy application source
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

# 3. Compile all 3 binaries using Go package cache mount (speeds up builds 10x)
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT}" \
      -o /out/server ./cmd/server && \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    go build -trimpath -ldflags="-s -w" -o /out/cli    ./cmd/cli

# Final minimal runtime image
FROM debian:bookworm-slim

WORKDIR /app

# ca-certificates for outbound TLS to the Gateway and object storage.
# curl for the container healthcheck.
# tzdata for Africa/Cairo timezone localization.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl tzdata && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r dawa24 && useradd -r -g dawa24 -d /app dawa24

COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/cli    /app/cli
COPY --from=builder /app/internal/ui/static /app/internal/ui/static

RUN chown -R dawa24:dawa24 /app
USER dawa24

EXPOSE 8080

CMD ["/app/server"]
