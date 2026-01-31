<p align="center">
  <img src="static/icon-192x192.png" alt="Sharm" width="96" height="96">
</p>

<h3 align="center">Sharm</h3>
<p align="center">Self-hosted ephemeral media sharing with auto-transcoding and rich embeds.</p>

---

Upload videos, audio, and images. Get shareable links that expire. Videos are auto-converted to AV1 and H264 for broad compatibility (Discord, browsers, etc). Shared links render with Open Graph and Twitter Card tags, so previews work when pasted into chat apps and social media.

Single-user, single-binary, single Docker container. SQLite for storage, FFmpeg for conversion.

## Deploy

Sharm needs to run on a server with a public domain for share links and embeds to work.

Create a `docker-compose.yml` on your server:

```yaml
services:
  sharm:
    image: ghcr.io/bnema/sharm:latest
    ports:
      - "7890:7890"
    environment:
      - DOMAIN=sharm.example.com
      - BEHIND_PROXY=true
    volumes:
      - sharm-data:/data
    restart: unless-stopped

volumes:
  sharm-data:
```

```bash
docker compose up -d
```

Point your reverse proxy at port 7890 and open `https://sharm.example.com`. On first launch you'll be prompted to create an account. Only one user can be registered.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | `localhost:7890` | Domain used in share URLs and embeds |
| `PORT` | `7890` | HTTP port |
| `MAX_UPLOAD_SIZE_MB` | `500` | Max upload size in MB |
| `DEFAULT_RETENTION_DAYS` | `7` | Days before shared links expire |
| `DATA_DIR` | `/data` | Where uploads, converted files, and the DB live |
| `BEHIND_PROXY` | `false` | Set to `true` when running behind a reverse proxy |
| `SECRET_KEY` | (auto-generated) | Key for signing session tokens. Generated and persisted to `DATA_DIR/.secret_key` if not set |

### Reverse Proxy

Nginx example:

```nginx
server {
    listen 80;
    server_name sharm.example.com;

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
