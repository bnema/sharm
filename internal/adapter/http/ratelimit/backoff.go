package ratelimit

import (
	"crypto/rand"
	"math"
	"math/big"
	"time"
)

type Backoff struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter bool
}

func NewBackoff(minDuration, maxDuration time.Duration, factor float64) *Backoff {
	return &Backoff{
		Min:    minDuration,
		Max:    maxDuration,
		Factor: factor,
		Jitter: true,
	}
}

func (b *Backoff) Duration(attempt int) time.Duration {
	if attempt <= 0 {
		return b.Min
	}

	duration := float64(b.Min) * pow(b.Factor, float64(attempt-1))

	if duration > float64(b.Max) {
		duration = float64(b.Max)
	}

	if b.Jitter {
		duration *= 0.5 + secureJitter()*0.5
	}

	return time.Duration(duration)
}

func pow(base, exp float64) float64 {
	result := 1.0
	for range int(exp) {
		result *= base
	}
	return result
}

func secureJitter() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return 1
	}
	return float64(n.Int64()) / float64(math.MaxInt64)
}

type LoginAttemptTracker struct {
	attempts map[string]int
}

func NewLoginAttemptTracker() *LoginAttemptTracker {
	return &LoginAttemptTracker{
		attempts: make(map[string]int),
	}
}

func (t *LoginAttemptTracker) GetFailedAttempts(clientID string) int {
	if attempts, exists := t.attempts[clientID]; exists {
		return attempts
	}
	return 0
}

func (t *LoginAttemptTracker) RecordFailure(clientID string) {
	t.attempts[clientID]++
}

func (t *LoginAttemptTracker) RecordSuccess(clientID string) {
	delete(t.attempts, clientID)
}
