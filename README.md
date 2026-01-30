# Sharm

Self-hosted, easy-to-deploy media sharing platform with temporary sharing links and social media embed support.

## Features

- **Video Upload & Conversion** - Automatic AV1/H264 conversion with FFmpeg
- **Temporary Sharing Links** - Videos expire after configurable retention period
- **Social Media Embeds** - Open Graph and Twitter Card tags for rich previews across platforms
- **Single-User Authentication** - Simple password-based login
- **Rate Limiting** - Brute-force protection with exponential backoff
- **Responsive UI** - Clean interface built with HTMX and templ
- **Easy Deployment** - Single Docker container with persistent volume
- **Thumbnail Generation** - Automatic video thumbnails

## Quick Start

### Using Docker Compose (Recommended)

1. **Clone the repository:**
   ```bash
   git clone https://github.com/bnema/sharm.git
   cd sharm
   ```

2. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env and set AUTH_SECRET to a strong password
   ```

3. **Start the service:**
   ```bash
   docker compose up -d
   ```

4. **Access the application:**
   - Open `http://localhost:7890` in your browser
   - Login with the password you set in `AUTH_SECRET`

## Building with Docker Buildx

Sharm uses a multi-stage Dockerfile for optimized images. Build with `docker buildx` for multi-platform support and better caching.

### Standard Build (Current Platform)

```bash
docker build -t sharm:latest .
```

### Multi-Platform Build (AMD64, ARM64)

Build for multiple architectures using buildx:

```bash
# Create a new builder instance (first time only)
docker buildx create --name multiarch --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t sharm:latest \
  --push \
  .

# Or build without pushing (to local images)
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t sharm:latest \
  --output type=image,push=false \
  .
```

### Build with BuildKit Cache

Use BuildKit's cache mounting for faster rebuilds:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --cache-from type=local,src=/tmp/.buildx-cache \
  --cache-to type=local,dest=/tmp/.buildx-cache \
  -t sharm:latest \
  .
```

### Build for Specific Platform

```bash
# Build for ARM64 (Raspberry Pi, AWS Graviton, etc.)
docker buildx build \
  --platform linux/arm64 \
  -t sharm:latest \
  .

# Build for AMD64 only
docker buildx build \
  --platform linux/amd64 \
  -t sharm:latest \
  .
```

### Using Make

The Makefile provides convenient targets for Docker builds:

```bash
# Build for current platform
make docker-build

# Build and push multi-platform image (AMD64, ARM64)
# Set REGISTRY in .env or as environment variable
export REGISTRY=your-registry.com
make docker-buildx-push

# Or use directly:
make docker-buildx-multi      # Build multi-platform locally
make docker-push              # Push to registry
make release                  # Full release build + push
```

The Makefile reads the `REGISTRY` variable from your `.env` file or environment:

```bash
# In .env:
REGISTRY=ghcr.io/yourusername

# Or as environment variable:
export REGISTRY=your-registry.com
make docker-buildx-push
```

### Build Options

```bash
# Build with custom tags
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/yourusername/sharm:latest \
  -t ghcr.io/yourusername/sharm:v1.0.0 \
  --push \
  .

# Build with build arguments
docker buildx build \
  --platform linux/amd64 \
  --build-arg GO_VERSION=1.25 \
  -t sharm:latest \
  .
```

## Configuration

Sharm is configured via environment variables:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP port to listen on | `7890` | No |
| `DOMAIN` | Domain for URLs and embeds | `localhost:7890` | No |
| `AUTH_SECRET` | Login password | - | **Yes** |
| `MAX_UPLOAD_SIZE_MB` | Maximum upload size | `500` | No |
| `DEFAULT_RETENTION_DAYS` | Default link expiration | `7` | No |
| `DATA_DIR` | Data storage path | `/data` | No |

### Environment File

Create a `.env` file:

```bash
# Server Configuration
PORT=7890
DOMAIN=your-domain.com

# Authentication (REQUIRED)
AUTH_SECRET=your-strong-password-here

# Upload Settings
MAX_UPLOAD_SIZE_MB=500
DEFAULT_RETENTION_DAYS=7

# Data Storage
DATA_DIR=/data
```

## Deployment

### Production Deployment

1. **Set a strong password:**
   ```bash
   # Generate a secure password
   openssl rand -base64 32
   ```

2. **Update docker-compose.yml:**
   ```yaml
   services:
     sharm:
       image: ghcr.io/yourusername/sharm:latest
       ports:
         - "7890:7890"
       environment:
         - DOMAIN=your-domain.com
         - AUTH_SECRET=<your-strong-password>
         - MAX_UPLOAD_SIZE_MB=500
         - DEFAULT_RETENTION_DAYS=7
       volumes:
         - sharm-data:/data
       restart: unless-stopped
   ```

3. **Behind a Reverse Proxy (Optional):**

   **Nginx Example:**
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

4. **Run with HTTPS:**
   ```bash
   docker compose up -d
   ```

### Volume Management

Data is persisted in a Docker volume at `/data`:
- `uploads/` - Temporary uploaded files
- `converted/` - Converted videos and thumbnails
- `media.json` - Media metadata database

**Backup the volume:**
```bash
docker run --rm \
  -v sharm-data:/data \
  -v $(pwd):/backup \
  ubuntu tar czf /backup/sharm-backup.tar.gz /data
```

## Development

### Quick Start with Make

The project includes a comprehensive Makefile for common tasks:

```bash
# Show all available targets
make help

# Development workflow
make deps          # Download dependencies
make generate      # Generate code (templ, mocks)
make build         # Build the binary
make test          # Run tests
make dev           # Run with hot reload
```

### Prerequisites

- Go 1.25+
- FFmpeg
- templ (templ/templ)
- Air (for hot reload, `cosmtrek/air`)
- mockery (for generating mocks, `vektra/mockery`)

### Local Development

**Using Make (recommended):**

```bash
# Set up environment
cp .env.example .env
# Edit .env with your settings

# Install dependencies and generate code
make deps generate

# Run with hot reload
make dev

# Or build and run manually
make build
make run
```

**Manual setup:**

1. **Install dependencies:**
   ```bash
   go mod download
   ```

2. **Generate templ code:**
   ```bash
   templ generate
   ```

3. **Generate mocks:**
   ```bash
   mockery
   ```

4. **Run with hot reload:**
   ```bash
   air
   ```

5. **Access the application:**
   ```
   http://localhost:7890
   ```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run with race detector
go test ./... -race
```

### Project Structure

```
sharm/
├── cmd/sharm/           # Application entry point
├── internal/
│   ├── adapter/         # External integrations (storage, converter, HTTP)
│   │   ├── http/        # HTTP handlers, middleware, rate limiting
│   │   ├── storage/     # JSON file storage
│   │   └── converter/   # FFmpeg converter
│   ├── domain/          # Business entities (Media, errors)
│   ├── port/            # Interfaces (Storage, Converter)
│   └── service/         # Business logic (MediaService, AuthService)
├── static/              # Static assets
├── Dockerfile           # Multi-stage build
├── docker-compose.yml   # Deployment configuration
└── .mockery.yml         # Mock generation config
```

### Architecture

Sharm follows **hexagonal/clean architecture** principles:

- **Domain Layer** - Core business entities (Media, errors)
- **Port Layer** - Interfaces for external dependencies
- **Adapter Layer** - Implementations of ports (JSON storage, FFmpeg, HTTP)
- **Service Layer** - Business logic orchestration

This design allows easy testing (using mocks) and swapping implementations.

## Security Features

- **Rate Limiting** - 5 login attempts per 15 minutes per IP
- **Exponential Backoff** - Increasing delays after failed attempts (500ms → 10s)
- **Temporary Links** - Videos expire after retention period
- **Non-Root User** - Container runs as unprivileged user
- **HTTPS Support** - Sets secure cookies when TLS is enabled

## Roadmap

- [ ] Multi-user support with per-user quotas
- [ ] S3-compatible storage backend
- [ ] Video transcoding profiles
- [ ] Batch upload support
- [ ] Webhook notifications on conversion completion
- [ ] Admin dashboard for monitoring

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- [Documentation](docs/)
- [Issue Tracker](https://github.com/bnema/sharm/issues)
- [Discussions](https://github.com/bnema/sharm/discussions)

## Acknowledgments

Built with:
- [Go](https://go.dev/) - Programming language
- [FFmpeg](https://ffmpeg.org/) - Video conversion
- [templ](https://templ.guide/) - HTML templating
- [HTMX](https://htmx.org/) - Dynamic UI interactions
- [testify](https://github.com/stretchr/testify) - Testing framework
