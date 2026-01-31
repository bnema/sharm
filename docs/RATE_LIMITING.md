# Rate Limiting and Backoff

## How It Works

Login attempts are rate-limited per IP with exponential backoff.

**Rate Limiter** (`internal/adapter/http/ratelimit/login.go`):
- 5 failed attempts allowed per 15-minute window
- IP is blocked for 30 minutes after exceeding the limit
- Uses `X-Forwarded-For` for proxied requests, falls back to `RemoteAddr`
- Old records are cleaned up every minute

**Backoff** (`internal/adapter/http/ratelimit/backoff.go`):
- Starts at 500ms, doubles each attempt, caps at 10s
- Adds jitter (50-100% of computed delay) to prevent timing attacks
- Resets on successful login

## Login Flow

1. Check empty password, return 400
2. Check rate limit, return 429 with `Retry-After` if blocked
3. Validate password
   - Invalid: record failure, apply backoff delay, return 401
   - Valid: reset counters, set cookie, redirect

## Configuration

Adjust in `internal/adapter/http/server.go`:

```go
rateLimiter := ratelimit.NewLoginRateLimiter(
    5,              // maxAttempts
    15*time.Minute, // windowDuration
    30*time.Minute, // blockDuration
)

backoff := ratelimit.NewBackoff(
    500*time.Millisecond, // min
    10*time.Second,       // max
    2.0,                  // factor
)
```

## Notes

- State is in-memory only. Restarting the container resets all rate limit counters.
- For multi-instance deployments, you would need a shared store (Redis, etc).
