FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY app/ ./app/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download && cd app && go mod tidy && go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/status .

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget && rm -rf /var/lib/apt/lists/*
RUN groupadd -r servicarr && useradd -r -g servicarr servicarr
COPY --from=build /out/status /usr/local/bin/status
COPY web/ ./web/
RUN mkdir -p /data && chown -R servicarr:servicarr /app /data
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/data"]
ENV TZ=Europe/London \
    PORT=4555 \
    DB_PATH=/data/uptime.db \
    POLL_SECONDS=60 \
    ENABLE_SCHEDULER=true \
    INSECURE_DEV=false \
    UNBLOCK_TOKEN="" \
    SERVER_HEALTH_URL="" \
    OVERSEERR_STATUS_URL="" \
    PLEX_BASE_URL="" \
    PLEX_TOKEN="" \
    AUTH_USER="admin" \
    AUTH_PASSWORD="" \
    AUTH_SECRET=""
EXPOSE 4555
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s CMD wget -qO- http://127.0.0.1:4555/api/check >/dev/null 2>&1 || exit 1
ENTRYPOINT ["/entrypoint.sh"]
