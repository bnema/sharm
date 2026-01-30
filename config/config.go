package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port                 int
	Domain               string
	AuthSecret           string
	MaxUploadSizeMB      int
	DefaultRetentionDays int
	DataDir              string
}

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

	authSecret := os.Getenv("AUTH_SECRET")
	if authSecret == "" {
		return nil, fmt.Errorf("AUTH_SECRET is required")
	}

	return &Config{
		Port:                 port,
		Domain:               getEnv("DOMAIN", "localhost:7890"),
		AuthSecret:           authSecret,
		MaxUploadSizeMB:      maxUploadSizeMB,
		DefaultRetentionDays: defaultRetentionDays,
		DataDir:              getEnv("DATA_DIR", "/data"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
