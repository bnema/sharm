<p align="center">
  <img src="static/icon-192x192.png" alt="Sharm" width="96" height="96">
</p>

<h3 align="center">Sharm</h3>
<p align="center">Self-hosted ephemeral media sharing with auto-transcoding and rich embeds.</p>

---

Upload videos, audio, and images. Get shareable links that expire. Videos are auto-converted to AV1 and H264 for broad compatibility (Discord, browsers, etc). Shared links render with Open Graph and Twitter Card tags, so previews work when pasted into chat apps and social media.

Single-user, single-binary, single Docker container. SQLite for storage, FFmpeg for conversion.

## Quick Start

```bash
git clone https://github.com/bnema/sharm.git
cd sharm
cp .env.example .env
# edit .env if you want to change defaults
docker compose up -d
```

Open `http://localhost:7890`. On first launch you'll be prompted to create an account (username and password). Only one user can be registered.

## Configuration

All config lives in environment variables (or a `.env` file):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `7890` | HTTP port |
| `DOMAIN` | `localhost:7890` | Domain used in share URLs and embeds |
| `MAX_UPLOAD_SIZE_MB` | `500` | Max upload size in MB |
| `DEFAULT_RETENTION_DAYS` | `7` | Days before shared links expire |
| `DATA_DIR` | `/data` | Where uploads, converted files, and the DB live |
| `BEHIND_PROXY` | `false` | Set to `true` when running behind a reverse proxy |
| `SECRET_KEY` | (auto-generated) | Key for signing session tokens. Generated and persisted to `DATA_DIR/.secret_key` if not set |

## Reverse Proxy

If running behind nginx or similar:

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:7890;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Development

Requires Go 1.25+, FFmpeg, and a few code generation tools (sqlc, templ, mockery).

```bash
cp .env.example .env
make deps generate   # download deps, generate code
make dev             # run with hot reload (requires air)
```

Or build and run manually:

```bash
make build
make run
```

Run tests:

```bash
make test
```

`make help` lists all available targets.

## Project Structure

```
internal/
  domain/       Core types: Media, User, Job, Probe
  port/         Interfaces (MediaStore, MediaConverter, JobQueue, etc.)
  adapter/
    http/       Handlers, middleware, templates, rate limiting
    storage/    SQLite implementation
    converter/  FFmpeg implementation
  service/      Business logic (MediaService, AuthService, Worker pool)
```

Follows hexagonal architecture. Domain and ports have no dependency on adapters. Swap SQLite for Postgres, or FFmpeg for another converter, without touching business logic.

## Docker Build

```bash
# current platform
make docker-build

# multi-platform (amd64 + arm64) and push
make docker-buildx-push
```

Set `REGISTRY` in `.env` or as an env var (defaults to `ghcr.io/bnema`).

## Contributing

```bash
make deps generate   # set up
make dev             # hack on it
make check           # fmt + vet + test before submitting
```

Mocks are auto-generated from `.mockery.yml`. Do not edit them by hand.
