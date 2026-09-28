package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type NotificationAPIConfig struct {
	DatabaseURL   string
	HTTPAddr      string
	PublicKeyPath string
	KeyID         string
	Issuer        string
}

func ReadNotificationAPIConfig() (*NotificationAPIConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}
	cfg := &NotificationAPIConfig{
		DatabaseURL:   strings.TrimSpace(os.Getenv("NOTIFICATION_DATABASE_URL")),
		HTTPAddr:      strings.TrimSpace(os.Getenv("NOTIFICATION_HTTP_ADDR")),
		PublicKeyPath: strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_PATH")),
		KeyID:         strings.TrimSpace(os.Getenv("JWT_KEY_ID")),
		Issuer:        strings.TrimSpace(os.Getenv("JWT_ISSUER")),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("NOTIFICATION_DATABASE_URL is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:8081"
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return nil, fmt.Errorf("invalid NOTIFICATION_HTTP_ADDR: %w", err)
	}
	if cfg.PublicKeyPath == "" {
		return nil, fmt.Errorf("JWT_PUBLIC_KEY_PATH is required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("JWT_KEY_ID is required")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("JWT_ISSUER is required")
	}
	return cfg, nil
}
