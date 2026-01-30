# =============================================================
# Stage 1: Fetch Go modules (cached layer)
# =============================================================
FROM golang:1.25-bookworm AS fetch-stage
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

# =============================================================
# Stage 2: Build Go binary
# =============================================================
FROM golang:1.25-bookworm AS build-stage
WORKDIR /app
COPY --from=fetch-stage /go/pkg/mod /go/pkg/mod
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -ldflags="-s -w" -o /sharm ./cmd/sharm

# =============================================================
# Stage 3: Final runtime image
# =============================================================
FROM ubuntu:24.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Non-root user
RUN useradd -m -s /bin/bash appuser

# Data directory structure (persisted via volume)
RUN mkdir -p /data/uploads /data/converted && \
    chown -R appuser:appuser /data

COPY --from=build-stage /sharm /usr/local/bin/sharm

USER appuser

# Default env (override at runtime)
ENV PORT=7890 \
    DOMAIN=localhost:7890 \
    MAX_UPLOAD_SIZE_MB=500 \
    DEFAULT_RETENTION_DAYS=7 \
    DATA_DIR=/data

EXPOSE 7890

# Mount point: contains uploads/, converted/, and media.json
VOLUME ["/data"]

ENTRYPOINT ["sharm"]
