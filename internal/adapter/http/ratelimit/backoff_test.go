package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff_Duration_Attempt0(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	duration := backoff.Duration(0)
	assert.Equal(t, 100*time.Millisecond, duration)
}

func TestBackoff_Duration_Attempt1(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	backoff.Jitter = false
	duration := backoff.Duration(1)
	assert.Equal(t, 100*time.Millisecond, duration)
}

func TestBackoff_Duration_Attempt2(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	backoff.Jitter = false
	duration := backoff.Duration(2)
	assert.Equal(t, 200*time.Millisecond, duration)
}

func TestBackoff_Duration_Attempt3(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	backoff.Jitter = false
	duration := backoff.Duration(3)
	assert.Equal(t, 400*time.Millisecond, duration)
}

func TestBackoff_Duration_CapsAtMax(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 500*time.Millisecond, 2.0)
	backoff.Jitter = false
	duration := backoff.Duration(10)
	assert.Equal(t, 500*time.Millisecond, duration)
}

func TestBackoff_Duration_WithJitter(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	backoff.Jitter = true

	durations := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		durations[i] = backoff.Duration(3)
	}

	expected := 400 * time.Millisecond
	minJitter := time.Duration(float64(expected) * 0.5)
	maxJitter := time.Duration(float64(expected) * 1.0)

	for _, d := range durations {
		assert.GreaterOrEqual(t, d, minJitter)
		assert.LessOrEqual(t, d, maxJitter)
	}
}

func TestBackoff_Duration_WithoutJitter(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	backoff.Jitter = false

	duration := backoff.Duration(3)
	assert.Equal(t, 400*time.Millisecond, duration)
}

func TestBackoff_Duration_NegativeAttempt(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)
	duration := backoff.Duration(-5)
	assert.Equal(t, 100*time.Millisecond, duration)
}

func TestPow(t *testing.T) {
	tests := []struct {
		name     string
		base     float64
		exp      float64
		expected float64
	}{
		{"2^0", 2.0, 0.0, 1.0},
		{"2^1", 2.0, 1.0, 2.0},
		{"2^2", 2.0, 2.0, 4.0},
		{"2^3", 2.0, 3.0, 8.0},
		{"3^2", 3.0, 2.0, 9.0},
		{"10^3", 10.0, 3.0, 1000.0},
		{"1.5^2", 1.5, 2.0, 2.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pow(tt.base, tt.exp)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestPow_LargeExponent(t *testing.T) {
	result := pow(2.0, 10.0)
	assert.Equal(t, 1024.0, result)
}

func TestPow_Decimals(t *testing.T) {
	result := pow(2.5, 2.0)
	expected := 2.5 * 2.5
	assert.InDelta(t, expected, result, 0.0001)
}

func TestNewBackoff_Defaults(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 5*time.Second, 2.0)

	assert.Equal(t, 100*time.Millisecond, backoff.Min)
	assert.Equal(t, 5*time.Second, backoff.Max)
	assert.Equal(t, 2.0, backoff.Factor)
	assert.True(t, backoff.Jitter)
}
