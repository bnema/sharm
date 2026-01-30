# Rate Limiting and Backoff Implementation

## Overview

This implementation adds rate limiting and exponential backoff with jitter to the login endpoint to prevent brute force attacks and protect against automated login attempts.

## Components

### 1. Rate Limiter (`internal/adapter/http/ratelimit/login.go`)

A simple in-memory rate limiter that tracks failed login attempts per IP address.

**Configuration:**
- **Max Attempts**: 5 failed attempts allowed
- **Window Duration**: 15 minutes (attempt counter resets after this period of inactivity)
- **Block Duration**: 30 minutes (IP is blocked after exceeding max attempts)

**Features:**
- Per-IP tracking using `X-Forwarded-For` header or `RemoteAddr`
- Automatic cleanup of old records (runs every minute)
- Thread-safe with mutex protection
- Returns remaining block duration when limit exceeded

### 2. Backoff Mechanism (`internal/adapter/http/ratelimit/backoff.go`)

Implements exponential backoff with jitter to delay responses to failed login attempts.

**Configuration:**
- **Min Delay**: 500ms
- **Max Delay**: 10 seconds
- **Factor**: 2.0 (exponential)
- **Jitter**: Enabled (randomizes delay by 50-100% to prevent timing attacks)

**Backoff Progression:**
- Attempt 1: 500ms
- Attempt 2: ~1s (with jitter)
- Attempt 3: ~2s (with jitter)
- Attempt 4: ~4s (with jitter)
- Attempt 5: ~8s (with jitter)
- Attempt 6+: ~10s (maxed out)

**Features:**
- Tracks failed attempts per IP
- Resets counter on successful login
- Follows AWS best practices for jitter implementation

### 3. Integration (`internal/adapter/http/auth.go`)

Updated `LoginHandler` to use rate limiting and backoff:

**Flow:**
1. Extract client IP (from `X-Forwarded-For` or `RemoteAddr`)
2. Check empty password → 400 Bad Request
3. Check rate limit → 429 Too Many Requests (if exceeded)
4. Validate password
   - If invalid: record failure, apply backoff delay, return 401
   - If valid: record success, reset counters, set cookie, redirect

**Error Messages:**
- Empty password: "Password is required"
- Invalid password: "Invalid password"
- Rate limited: "Too many attempts. Try again in X seconds/minutes/hours"

## Testing

### Manual Testing

1. **Test Valid Login:**
   ```bash
   curl -c cookies.txt -X POST http://localhost:7890/login \
     -d "password=test123" -v
   ```
   Expected: 302 redirect to `/`, cookie set

2. **Test Invalid Password (with backoff):**
   ```bash
   time curl -X POST http://localhost:7890/login \
     -d "password=wrong" -v
   ```
   Expected: 401 Unauthorized, notice increasing delays

3. **Test Rate Limiting:**
   ```bash
   for i in {1..6}; do
     curl -X POST http://localhost:7890/login -d "password=wrong" \
       -w "\nStatus: %{http_code}\n"
   done
   ```
   Expected: First 5 get 401, 6th gets 429 with Retry-After header

### Automated Testing Script

```bash
#!/bin/bash

echo "Testing rate limiting and backoff..."
echo

# Test 1: Valid login
echo "Test 1: Valid login"
curl -c cookies.txt -X POST http://localhost:7890/login \
  -d "password=test123" \
  -w "\nStatus: %{http_code}\n" \
  -s -o /dev/null
echo

# Test 2: Empty password
echo "Test 2: Empty password"
curl -X POST http://localhost:7890/login \
  -d "password=" \
  -w "\nStatus: %{http_code}\n" \
  -s
echo

# Test 3: Invalid password (with backoff timing)
echo "Test 3: Invalid password attempts (note increasing delays)"
for i in {1..5}; do
  echo "Attempt $i:"
  time curl -X POST http://localhost:7890/login \
    -d "password=wrong" \
    -w "\nStatus: %{http_code}\n" \
    -s -o /dev/null
  echo
done

# Test 4: Rate limit exceeded
echo "Test 4: Rate limit exceeded (6th attempt)"
curl -X POST http://localhost:7890/login \
  -d "password=wrong" \
  -w "\nStatus: %{http_code}\n" \
  -s -D - | grep -E "HTTP|Retry-After"
echo

# Test 5: Wait for block to expire (optional)
echo "Test 5: Waiting 30 minutes for block to expire..."
echo "(Skipping in demo - would need to wait)"
```

## Configuration

To adjust the rate limiting and backoff parameters, modify `internal/adapter/http/server.go`:

```go
rateLimiter := ratelimit.NewLoginRateLimiter(
    5,                  // maxAttempts
    15*time.Minute,     // windowDuration
    30*time.Minute,     // blockDuration
)

backoff := ratelimit.NewBackoff(
    500*time.Millisecond,  // min
    10*time.Second,        // max
    2.0,                   // factor
)
```

## Security Considerations

1. **IP-based Tracking**: Uses `X-Forwarded-For` header for proxied requests, falls back to `RemoteAddr`
2. **Jitter**: Prevents timing attacks and thundering herd problems
3. **Memory Cleanup**: Old records are automatically cleaned up every minute
4. **No Persistent Storage**: Rate limit state is lost on container restart (acceptable for single-user deployment)
5. **Exponential Backoff**: Makes brute force attacks impractical by increasing time cost exponentially

## Production Recommendations

For production deployments with multiple instances:

1. **Use Redis Backend**: Replace in-memory storage with Redis for distributed rate limiting
2. **Add CAPTCHA**: Implement hCaptcha or Cloudflare Turnstile after 3 failed attempts
3. **Account Lockout**: Consider adding permanent account lockout after excessive failures
4. **Monitoring**: Add metrics for rate limit violations and failed login attempts
5. **Alerts**: Set up alerts for repeated rate limit violations from same IP

## References

Based on Context7 documentation:
- [jpillora/backoff](https://context7.com/jpillora/backoff/llms.txt) - Exponential backoff with jitter
- [ajiwo/ratelimit](https://context7.com/ajiwo/ratelimit/llms.txt) - Rate limiting strategies (implemented custom version for simplicity)
