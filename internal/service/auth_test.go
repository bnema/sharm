package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuthService_ValidatePassword(t *testing.T) {
	secret := "test-secret-123"
	service := NewAuthService(secret)

	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{
			name:     "correct password",
			password: secret,
			want:     true,
		},
		{
			name:     "incorrect password",
			password: "wrong-password",
			want:     false,
		},
		{
			name:     "empty password",
			password: "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ValidatePassword(tt.password)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuthService_GenerateToken(t *testing.T) {
	secret := "test-secret-123"
	service := NewAuthService(secret)

	t.Run("generates token with timestamp:signature format", func(t *testing.T) {
		token := service.GenerateToken()
		parts := strings.Split(token, ":")
		assert.Len(t, parts, 2, "token should have format timestamp:signature")

		_, err := strconv.ParseInt(parts[0], 10, 64)
		assert.NoError(t, err, "first part should be valid timestamp")
	})

	t.Run("signature is valid HMAC-SHA256", func(t *testing.T) {
		token := service.GenerateToken()
		parts := strings.Split(token, ":")
		timestamp, signature := parts[0], parts[1]

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp))
		expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

		assert.Equal(t, expectedSignature, signature, "signature should be valid HMAC-SHA256")
	})

	t.Run("different tokens have different timestamps", func(t *testing.T) {
		token1 := service.GenerateToken()
		time.Sleep(1 * time.Second)
		token2 := service.GenerateToken()

		parts1 := strings.Split(token1, ":")
		parts2 := strings.Split(token2, ":")

		assert.NotEqual(t, parts1[0], parts2[0], "timestamps should be different")
	})
}

func TestAuthService_ValidateToken(t *testing.T) {
	secret := "test-secret-123"
	service := NewAuthService(secret)

	t.Run("returns nil for valid token", func(t *testing.T) {
		token := service.GenerateToken()
		err := service.ValidateToken(token)
		assert.NoError(t, err)
	})

	t.Run("returns ErrInvalidToken for malformed format", func(t *testing.T) {
		tests := []struct {
			name  string
			token string
		}{
			{"missing colon", "timestamponly"},
			{"empty string", ""},
			{"multiple colons", "ts:sig:extra"},
			{"only colon", ":"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := service.ValidateToken(tt.token)
				assert.ErrorIs(t, err, ErrInvalidToken)
			})
		}
	})

	t.Run("returns ErrInvalidToken for wrong signature", func(t *testing.T) {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		wrongSignature := base64.URLEncoding.EncodeToString([]byte("wrong"))
		token := timestamp + ":" + wrongSignature

		err := service.ValidateToken(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrExpiredToken for old tokens", func(t *testing.T) {
		oldTimestamp := time.Now().Add(-8 * 24 * time.Hour).Unix()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(strconv.FormatInt(oldTimestamp, 10)))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := strconv.FormatInt(oldTimestamp, 10) + ":" + signature

		err := service.ValidateToken(token)
		assert.ErrorIs(t, err, ErrExpiredToken)
	})

	t.Run("returns nil for token within expiration window", func(t *testing.T) {
		recentTimestamp := time.Now().Add(-6 * 24 * time.Hour).Unix()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(strconv.FormatInt(recentTimestamp, 10)))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := strconv.FormatInt(recentTimestamp, 10) + ":" + signature

		err := service.ValidateToken(token)
		assert.NoError(t, err)
	})

	t.Run("handles invalid timestamp in token", func(t *testing.T) {
		invalidTimestamp := "not-a-number"

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(invalidTimestamp))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := invalidTimestamp + ":" + signature

		err := service.ValidateToken(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})
}
