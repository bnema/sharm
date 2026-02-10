package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginRateLimiter_Check_FirstAttempt(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute, 5*time.Minute)

	allowed, duration := limiter.Check("client1")

	assert.True(t, allowed)
	assert.Equal(t, time.Duration(0), duration)
}

func TestLoginRateLimiter_Check_SubsequentAttempts(t *testing.T) {
	limiter := NewLoginRateLimiter(5, 1*time.Minute, 5*time.Minute)

	for i := 0; i < 5; i++ {
		allowed, _ := limiter.Check("client1")
		assert.True(t, allowed)
	}
}

func TestLoginRateLimiter_Check_BlocksAfterMaxAttempts(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute, 5*time.Minute)

	limiter.Check("client1")
	limiter.Check("client1")
	limiter.Check("client1")

	allowed, duration := limiter.Check("client1")

	assert.False(t, allowed)
	assert.Equal(t, 5*time.Minute, duration)
}

func TestLoginRateLimiter_Check_RemainingBlockDuration(t *testing.T) {
	limiter := NewLoginRateLimiter(2, 1*time.Minute, 10*time.Minute)

	limiter.Check("client1")
	limiter.Check("client1")

	_, _ = limiter.Check("client1")

	time.Sleep(100 * time.Millisecond)

	allowed, remaining := limiter.Check("client1")

	assert.False(t, allowed)
	assert.Greater(t, remaining, 9*time.Minute)
	assert.LessOrEqual(t, remaining, 10*time.Minute)
}

func TestLoginRateLimiter_Check_ResetsAfterWindow(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 100*time.Millisecond, 5*time.Minute)

	limiter.Check("client1")
	limiter.Check("client1")
	limiter.Check("client1")

	time.Sleep(150 * time.Millisecond)

	allowed, _ := limiter.Check("client1")

	assert.True(t, allowed)
}

func TestLoginRateLimiter_Check_NewClient(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute, 5*time.Minute)

	limiter.Check("client1")
	limiter.Check("client1")
	limiter.Check("client1")
	limiter.Check("client1")

	allowed, _ := limiter.Check("client2")

	assert.True(t, allowed)
}

func TestLoginRateLimiter_Reset(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute, 5*time.Minute)

	limiter.Check("client1")
	limiter.Check("client1")
	limiter.Check("client1")

	limiter.Reset("client1")

	allowed, _ := limiter.Check("client1")

	assert.True(t, allowed)
}

func TestLoginRateLimiter_Reset_NonExistentClient(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 1*time.Minute, 5*time.Minute)

	limiter.Reset("nonexistent")

	allowed, _ := limiter.Check("nonexistent")

	assert.True(t, allowed)
}

func TestLoginRateLimiter_cleanup_RemovesOldRecords(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 50*time.Millisecond, 100*time.Millisecond)

	limiter.Check("client1")

	time.Sleep(110 * time.Millisecond)

	limiter.mu.Lock()
	now := time.Now()
	for clientID, record := range limiter.attempts {
		if now.Sub(record.LastAttempt) > limiter.windowDuration*2 && now.After(record.BlockedUntil) {
			delete(limiter.attempts, clientID)
		}
	}
	limiter.mu.Unlock()

	limiter.mu.RLock()
	_, exists := limiter.attempts["client1"]
	limiter.mu.RUnlock()

	assert.False(t, exists)
}

func TestLoginRateLimiter_cleanup_PreservesActiveRecords(t *testing.T) {
	limiter := NewLoginRateLimiter(3, 100*time.Millisecond, 200*time.Millisecond)

	limiter.Check("client1")

	time.Sleep(50 * time.Millisecond)

	limiter.mu.RLock()
	_, exists := limiter.attempts["client1"]
	limiter.mu.RUnlock()

	assert.True(t, exists)
}

func TestLoginRateLimiter_Check_BlockedClient(t *testing.T) {
	limiter := NewLoginRateLimiter(2, 1*time.Minute, 200*time.Millisecond)

	limiter.Check("client1")
	limiter.Check("client1")

	_, _ = limiter.Check("client1")

	allowed, _ := limiter.Check("client1")

	assert.False(t, allowed)
}

func TestLoginRateLimiter_Check_BlockExpires(t *testing.T) {
	limiter := NewLoginRateLimiter(2, 1*time.Minute, 100*time.Millisecond)

	limiter.Check("client1")
	limiter.Check("client1")

	_, _ = limiter.Check("client1")

	time.Sleep(150 * time.Millisecond)

	limiter.Reset("client1")

	allowed, _ := limiter.Check("client1")

	assert.True(t, allowed)
}

func TestNewLoginRateLimiter(t *testing.T) {
	limiter := NewLoginRateLimiter(5, 2*time.Minute, 10*time.Minute)

	assert.NotNil(t, limiter)
	assert.Equal(t, 5, limiter.maxAttempts)
	assert.Equal(t, 2*time.Minute, limiter.windowDuration)
	assert.Equal(t, 10*time.Minute, limiter.blockDuration)
}

func TestLoginRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewLoginRateLimiter(100, 1*time.Minute, 5*time.Minute)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				limiter.Check("concurrent-client")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	limiter.mu.RLock()
	record, exists := limiter.attempts["concurrent-client"]
	limiter.mu.RUnlock()

	require.True(t, exists)
	assert.Equal(t, 100, record.Count)
}
