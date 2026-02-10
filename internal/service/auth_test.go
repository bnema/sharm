package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
)

type mockUserStore struct {
	user          *domain.User
	hasUser       bool
	createUserErr error
	getUserErr    error
}

func (m *mockUserStore) HasUser() (bool, error) {
	return m.hasUser, m.createUserErr
}

func (m *mockUserStore) GetUser(_ string) (*domain.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	return m.user, nil
}

func (m *mockUserStore) GetFirstUser() (*domain.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	return m.user, nil
}

func (m *mockUserStore) GetUserByID(_ int64) (*domain.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	return m.user, nil
}

func (m *mockUserStore) CreateUser(username, passwordHash string) error {
	if m.createUserErr != nil {
		return m.createUserErr
	}
	m.user = &domain.User{
		ID:           1,
		Username:     username,
		PasswordHash: passwordHash,
	}
	m.hasUser = true
	return nil
}

func (m *mockUserStore) UpdatePassword(_ int64, passwordHash string) error {
	if m.user != nil {
		m.user.PasswordHash = passwordHash
	}
	return nil
}

func TestAuthService_HasUser(t *testing.T) {
	t.Run("returns false when no user exists", func(t *testing.T) {
		store := &mockUserStore{hasUser: false}
		svc := NewAuthService(store, "test-secret-key")
		hasUser, err := svc.HasUser()
		assert.NoError(t, err)
		assert.False(t, hasUser)
	})

	t.Run("returns true when user exists", func(t *testing.T) {
		store := &mockUserStore{hasUser: true}
		svc := NewAuthService(store, "test-secret-key")
		hasUser, err := svc.HasUser()
		assert.NoError(t, err)
		assert.True(t, hasUser)
	})
}

func TestAuthService_CreateUser(t *testing.T) {
	t.Run("creates user successfully", func(t *testing.T) {
		store := &mockUserStore{hasUser: false}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.CreateUser("admin", "P@ssw0rd123")
		assert.NoError(t, err)
		assert.True(t, store.hasUser)
	})

	t.Run("returns error when user already exists", func(t *testing.T) {
		store := &mockUserStore{hasUser: true}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.CreateUser("admin", "P@ssw0rd123")
		assert.ErrorIs(t, err, ErrUserExists)
	})
}

func TestAuthService_ValidatePassword(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("P@ssw0rd123"), bcrypt.DefaultCost)

	t.Run("validates correct password", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.ValidatePassword("admin", "P@ssw0rd123")
		assert.NoError(t, err)
	})

	t.Run("returns error for wrong password", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.ValidatePassword("admin", "wrongpassword")
		assert.ErrorIs(t, err, ErrWrongPassword)
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		store := &mockUserStore{getUserErr: errors.New("not found")}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.ValidatePassword("nonexistent", "password")
		assert.ErrorIs(t, err, ErrInvalidCreds)
	})
}

func TestAuthService_GenerateToken(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("P@ssw0rd123"), bcrypt.DefaultCost)

	t.Run("generates token with timestamp:userID:signature format", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		token, err := svc.GenerateToken("admin")
		assert.NoError(t, err)
		parts := strings.Split(token, ":")
		assert.Len(t, parts, 3, "token should have format timestamp:userID:signature")

		_, err = strconv.ParseInt(parts[0], 10, 64)
		assert.NoError(t, err, "first part should be valid timestamp")
		_, err = strconv.ParseInt(parts[1], 10, 64)
		assert.NoError(t, err, "second part should be valid user ID")
	})

	t.Run("signature is valid HMAC-SHA256 with secret key", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		secretKey := "test-secret-key"
		svc := NewAuthService(store, secretKey)
		token, err := svc.GenerateToken("admin")
		assert.NoError(t, err)

		parts := strings.Split(token, ":")
		timestamp, userID, signature := parts[0], parts[1], parts[2]

		mac := hmac.New(sha256.New, []byte(secretKey))
		mac.Write([]byte(timestamp + ":" + userID))
		expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

		assert.Equal(t, expectedSignature, signature, "signature should be valid HMAC-SHA256")
	})

	t.Run("different tokens have different timestamps", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		token1, _ := svc.GenerateToken("admin")
		time.Sleep(1 * time.Second)
		token2, _ := svc.GenerateToken("admin")

		parts1 := strings.Split(token1, ":")
		parts2 := strings.Split(token2, ":")

		assert.NotEqual(t, parts1[0], parts2[0], "timestamps should be different")
	})
}

func TestAuthService_ValidateToken(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("P@ssw0rd123"), bcrypt.DefaultCost)

	t.Run("returns nil for valid token", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		token, _ := svc.GenerateToken("admin")
		user, err := svc.ValidateToken(token)
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "admin", user.Username)
	})

	t.Run("returns ErrInvalidToken for malformed format", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")

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
				_, err := svc.ValidateToken(tt.token)
				assert.ErrorIs(t, err, ErrInvalidToken)
			})
		}
	})

	t.Run("returns ErrInvalidToken for wrong signature", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")

		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		wrongSignature := base64.URLEncoding.EncodeToString([]byte("wrong"))
		token := timestamp + ":" + wrongSignature

		_, err := svc.ValidateToken(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns ErrExpiredToken for old tokens", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		secretKey := "test-secret-key"
		svc := NewAuthService(store, secretKey)

		oldTimestamp := time.Now().Add(-8 * 24 * time.Hour).Unix()
		userID := "1"
		mac := hmac.New(sha256.New, []byte(secretKey))
		mac.Write([]byte(strconv.FormatInt(oldTimestamp, 10) + ":" + userID))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := strconv.FormatInt(oldTimestamp, 10) + ":" + userID + ":" + signature

		_, err := svc.ValidateToken(token)
		assert.ErrorIs(t, err, ErrExpiredToken)
	})

	t.Run("returns nil for token within expiration window", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		secretKey := "test-secret-key"
		svc := NewAuthService(store, secretKey)

		recentTimestamp := time.Now().Add(-6 * 24 * time.Hour).Unix()
		userID := "1"
		mac := hmac.New(sha256.New, []byte(secretKey))
		mac.Write([]byte(strconv.FormatInt(recentTimestamp, 10) + ":" + userID))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := strconv.FormatInt(recentTimestamp, 10) + ":" + userID + ":" + signature

		user, err := svc.ValidateToken(token)
		assert.NoError(t, err)
		assert.NotNil(t, user)
	})

	t.Run("handles invalid timestamp in token", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")

		invalidTimestamp := "not-a-number"

		mac := hmac.New(sha256.New, passwordHash)
		mac.Write([]byte(invalidTimestamp))
		signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
		token := invalidTimestamp + ":" + signature

		_, err := svc.ValidateToken(token)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})
}

func TestAuthService_ChangePassword(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("P@ssw0rd123"), bcrypt.DefaultCost)

	t.Run("changes password successfully", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.ChangePassword("admin", "P@ssw0rd123", "N3wP@ssw0rd!")
		assert.NoError(t, err)
		assert.NotEqual(t, string(passwordHash), store.user.PasswordHash)
	})

	t.Run("returns error for wrong old password", func(t *testing.T) {
		store := &mockUserStore{
			user: &domain.User{
				ID:           1,
				Username:     "admin",
				PasswordHash: string(passwordHash),
			},
		}
		svc := NewAuthService(store, "test-secret-key")
		err := svc.ChangePassword("admin", "wrongpassword", "N3wP@ssw0rd!")
		assert.ErrorIs(t, err, ErrWrongPassword)
	})
}
