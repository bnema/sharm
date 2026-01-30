package ratelimit

import (
	"math/rand"
	"time"
)

type Backoff struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter bool
}

func NewBackoff(min, max time.Duration, factor float64) *Backoff {
	return &Backoff{
		Min:    min,
		Max:    max,
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
		duration = duration * (0.5 + rand.Float64()*0.5)
	}

	return time.Duration(duration)
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
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
