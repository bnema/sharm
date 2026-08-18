package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                 int
	Domain               string
	MaxUploadSizeMB      int
	DefaultRetentionDays int
	DataDir              string
	SecretKey            string
	BehindProxy          bool
	TrustedProxyCIDRs    []*net.IPNet
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

	secretKey := getEnv("SECRET_KEY", getEnv("AUTH_SECRET", ""))
	if secretKey == "" {
		dataDir := getEnv("DATA_DIR", "/data")
		secretKeyFile := filepath.Join(dataDir, ".secret_key")

		if keyBytes, err := os.ReadFile(secretKeyFile); err == nil {
			secretKey = string(keyBytes)
		} else {
			secretKey = generateSecretKey()
			if err := os.MkdirAll(dataDir, 0750); err == nil {
				_ = os.WriteFile(secretKeyFile, []byte(secretKey), 0600)
			}
		}
	}

	behindProxy := getEnv("BEHIND_PROXY", "false") == "true"
	trustedProxyCIDRs, err := parseCIDRs(getEnv("TRUSTED_PROXY_CIDRS", "127.0.0.0/8,::1/128"))
	if err != nil {
		return nil, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS: %w", err)
	}

	return &Config{
		Port:                 port,
		Domain:               getEnv("DOMAIN", "localhost:7890"),
		MaxUploadSizeMB:      maxUploadSizeMB,
		DefaultRetentionDays: defaultRetentionDays,
		DataDir:              getEnv("DATA_DIR", "/data"),
		SecretKey:            secretKey,
		BehindProxy:          behindProxy,
		TrustedProxyCIDRs:    trustedProxyCIDRs,
	}, nil
}

func parseCIDRs(value string) ([]*net.IPNet, error) {
	parts := strings.Split(value, ",")
	cidrs := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, err
		}
		cidrs = append(cidrs, network)
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("at least one CIDR is required")
	}
	return cidrs, nil
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
