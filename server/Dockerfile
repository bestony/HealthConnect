# syntax=docker/dockerfile:1

# healthd links DuckDB static libraries with cgo.
# The pre-built libraries exist for linux/amd64 and linux/arm64.
# The link step needs g++ and several gigabytes of memory.

FROM golang:1.27.3-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY server/ .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux \
    go build -mod=readonly -buildvcs=false -trimpath \
        -ldflags='-s -w' \
        -o /out/healthd ./cmd/healthd

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl libstdc++6 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 65532 healthd \
    && useradd --uid 65532 --gid healthd \
        --home-dir /var/lib/healthconnect --no-create-home \
        --shell /usr/sbin/nologin healthd \
    && mkdir -p /var/lib/healthconnect \
    && chown healthd:healthd /var/lib/healthconnect

COPY --from=build --chmod=755 /out/healthd /usr/local/bin/healthd
COPY --chmod=755 server/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

WORKDIR /var/lib/healthconnect

ENV HTTP_ADDR=0.0.0.0:8080 \
    DATA_DIR=/var/lib/healthconnect \
    LOG_LEVEL=info \
    MCP_ENABLED=true

EXPOSE 8080
VOLUME ["/var/lib/healthconnect"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["curl", "-fsS", "http://127.0.0.1:8080/v1/healthz"]

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
