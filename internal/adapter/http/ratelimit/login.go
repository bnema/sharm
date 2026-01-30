package ratelimit

import (
	"sync"
	"time"
)

type AttemptRecord struct {
	Count        int
	LastAttempt  time.Time
	BlockedUntil time.Time
}

type LoginRateLimiter struct {
	mu             sync.RWMutex
	attempts       map[string]*AttemptRecord
	maxAttempts    int
	windowDuration time.Duration
	blockDuration  time.Duration
}

func NewLoginRateLimiter(maxAttempts int, windowDuration, blockDuration time.Duration) *LoginRateLimiter {
	limiter := &LoginRateLimiter{
		attempts:       make(map[string]*AttemptRecord),
		maxAttempts:    maxAttempts,
		windowDuration: windowDuration,
		blockDuration:  blockDuration,
	}

	go limiter.cleanup()

	return limiter
}

func (r *LoginRateLimiter) Check(clientID string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	record, exists := r.attempts[clientID]

	if !exists {
		record = &AttemptRecord{
			Count:       0,
			LastAttempt: now,
		}
		r.attempts[clientID] = record
	}

	if now.Before(record.BlockedUntil) {
		remaining := record.BlockedUntil.Sub(now)
		return false, remaining
	}

	if now.Sub(record.LastAttempt) > r.windowDuration {
		record.Count = 0
	}

	record.Count++
	record.LastAttempt = now

	if record.Count > r.maxAttempts {
		record.BlockedUntil = now.Add(r.blockDuration)
		remaining := r.blockDuration
		return false, remaining
	}

	return true, 0
}

func (r *LoginRateLimiter) Reset(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.attempts, clientID)
}

func (r *LoginRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		now := time.Now()

		for clientID, record := range r.attempts {
			if now.Sub(record.LastAttempt) > r.windowDuration*2 && now.After(record.BlockedUntil) {
				delete(r.attempts, clientID)
			}
		}

		r.mu.Unlock()
	}
}
