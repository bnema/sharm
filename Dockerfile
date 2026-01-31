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

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /sharm ./cmd/sharm

# =============================================================
# Stage 3: Final runtime image (Alpine Linux)
# =============================================================
FROM alpine:3.23

# Install FFmpeg and ca-certificates
RUN apk add --no-cache ffmpeg ca-certificates

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Data directory structure (persisted via volume)
# uploads/ - original uploaded files
# converted/ - transcoded media files
# sharm.db - SQLite database (WAL mode)
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

# Mount point: contains uploads/, converted/, and sharm.db
VOLUME ["/data"]

ENTRYPOINT ["sharm"]
