package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrExpiredToken    = errors.New("expired token")
	ErrInvalidCreds    = errors.New("invalid credentials")
	ErrUserExists      = errors.New("user already exists")
	ErrUserNotFound    = errors.New("user not found")
	ErrWrongPassword   = errors.New("wrong password")
	ErrWeakPassword    = errors.New("password does not meet requirements")
	ErrInvalidUsername = errors.New("invalid username")
)

func validateUsername(username string) error {
	if len(username) < 3 {
		return fmt.Errorf("must be at least 3 characters")
	}
	if len(username) > 50 {
		return fmt.Errorf("must be at most 50 characters")
	}
	for _, r := range username {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return fmt.Errorf("must contain only letters, numbers, underscores, and hyphens")
		}
	}
	return nil
}

func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("must be at least 8 characters")
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasNumber {
		missing = append(missing, "number")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}

	if len(missing) > 0 {
		return fmt.Errorf("must contain at least one %s", formatMissingRequirements(missing))
	}

	return nil
}

func formatMissingRequirements(missing []string) string {
	if len(missing) == 1 {
		return missing[0]
	}
	if len(missing) == 2 {
		return missing[0] + " and " + missing[1]
	}

	result := ""
	for i, req := range missing {
		if i == len(missing)-1 {
			result += ", and " + req
		} else if i > 0 {
			result += ", " + req
		} else {
			result = req
		}
	}
	return result
}

type AuthService struct {
	store     port.UserStore
	secretKey string
}

func NewAuthService(store port.UserStore, secretKey string) *AuthService {
	return &AuthService{
		store:     store,
		secretKey: secretKey,
	}
}

func (s *AuthService) HasUser() (bool, error) {
	return s.store.HasUser()
}

func (s *AuthService) CreateUser(username, password string) error {
	hasUser, err := s.store.HasUser()
	if err != nil {
		return err
	}
	if hasUser {
		return ErrUserExists
	}

	if validateErr := validateUsername(username); validateErr != nil {
		return fmt.Errorf("%w: %w", ErrInvalidUsername, validateErr)
	}

	if validateErr := validatePasswordStrength(password); validateErr != nil {
		return validateErr
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.store.CreateUser(username, string(passwordHash))
}

func (s *AuthService) ValidatePassword(username, password string) error {
	user, err := s.store.GetUser(username)
	if err != nil {
		return ErrInvalidCreds
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return ErrWrongPassword
	}

	return nil
}

func (s *AuthService) GenerateToken(username string) (string, error) {
	user, err := s.store.GetUser(username)
	if err != nil {
		return "", err
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	userID := strconv.FormatInt(user.ID, 10)
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write([]byte(timestamp + ":" + userID))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return timestamp + ":" + userID + ":" + signature, nil
}

func (s *AuthService) ValidateToken(token string) (*domain.User, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	timestamp, userIDStr, signature := parts[0], parts[1], parts[2]

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.store.GetUserByID(userID)
	if err != nil {
		return nil, ErrInvalidToken
	}

	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write([]byte(timestamp + ":" + userIDStr))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, ErrInvalidToken
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, ErrInvalidToken
	}

	expirationTime := time.Unix(ts, 0).Add(7 * 24 * time.Hour)
	if time.Now().After(expirationTime) {
		return nil, ErrExpiredToken
	}

	return user, nil
}

func (s *AuthService) ChangePassword(username, oldPassword, newPassword string) error {
	user, err := s.store.GetUser(username)
	if err != nil {
		return err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword))
	if err != nil {
		return ErrWrongPassword
	}

	if validateErr := validatePasswordStrength(newPassword); validateErr != nil {
		return validateErr
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.store.UpdatePassword(user.ID, string(passwordHash))
}
