package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type AuthService struct {
	secret string
}

func NewAuthService(secret string) *AuthService {
	return &AuthService{
		secret: secret,
	}
}

func (s *AuthService) ValidatePassword(password string) bool {
	return password == s.secret
}

func (s *AuthService) GenerateToken() string {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte(timestamp))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return timestamp + ":" + signature
}

func (s *AuthService) ValidateToken(token string) error {
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return ErrInvalidToken
	}

	timestamp, signature := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte(timestamp))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return ErrInvalidToken
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidToken
	}

	expirationTime := time.Unix(ts, 0).Add(7 * 24 * time.Hour)
	if time.Now().After(expirationTime) {
		return ErrExpiredToken
	}

	return nil
}
