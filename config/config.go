package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Port                 int
	Domain               string
	MaxUploadSizeMB      int
	DefaultRetentionDays int
	DataDir              string
	SecretKey            string
	BehindProxy          bool
}

const (
	dataDirPerms   = 0o750
	secretFilePerm = 0o600
)

func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "7890"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	maxUploadSizeMB, err := strconv.Atoi(getEnv("MAX_UPLOAD_SIZE_MB", "500"))
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_UPLOAD_SIZE_MB: %w", err)
	}

	defaultRetentionDays, err := strconv.Atoi(getEnv("DEFAULT_RETENTION_DAYS", "7"))
	if err != nil {
		return nil, fmt.Errorf("invalid DEFAULT_RETENTION_DAYS: %w", err)
	}

	secretKey := getEnv("SECRET_KEY", getEnv("AUTH_SECRET", ""))
	if secretKey == "" {
		dataDir := getEnv("DATA_DIR", "/data")
		secretKeyFile := filepath.Join(dataDir, ".secret_key")

		if keyBytes, err := os.ReadFile(secretKeyFile); err == nil {
			secretKey = string(keyBytes)
		} else {
			secretKey = generateSecretKey()
			if err := os.MkdirAll(dataDir, dataDirPerms); err == nil {
				_ = os.WriteFile(secretKeyFile, []byte(secretKey), secretFilePerm)
			}
		}
	}

	behindProxy := getEnv("BEHIND_PROXY", "false") == "true"

	return &Config{
		Port:                 port,
		Domain:               getEnv("DOMAIN", "localhost:7890"),
		MaxUploadSizeMB:      maxUploadSizeMB,
		DefaultRetentionDays: defaultRetentionDays,
		DataDir:              getEnv("DATA_DIR", "/data"),
		SecretKey:            secretKey,
		BehindProxy:          behindProxy,
	}, nil
}

func generateSecretKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("failed to generate secret key: %w", err))
	}
	return base64.StdEncoding.EncodeToString(b)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
