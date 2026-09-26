package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"FeedFlow/internal/auth"
)

type JWTConfig struct {
	PrivateKeyPath string
	PublicKeyPath  string
	KeyID          string
	Issuer         string
	AccessTokenTTL time.Duration
}

func ReadJWTConfig() (*JWTConfig, error) {
	cfg := &JWTConfig{
		PrivateKeyPath: strings.TrimSpace(os.Getenv("JWT_PRIVATE_KEY_PATH")),
		PublicKeyPath:  strings.TrimSpace(os.Getenv("JWT_PUBLIC_KEY_PATH")),
		KeyID:          strings.TrimSpace(os.Getenv("JWT_KEY_ID")),
		Issuer:         strings.TrimSpace(os.Getenv("JWT_ISSUER")),
	}
	if cfg.PrivateKeyPath == "" || cfg.PublicKeyPath == "" {
		return nil, fmt.Errorf("JWT_PRIVATE_KEY_PATH and JWT_PUBLIC_KEY_PATH are required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("JWT_KEY_ID is required")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("JWT_ISSUER is required")
	}
	var err error
	cfg.AccessTokenTTL, err = readDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	if cfg.AccessTokenTTL < time.Second || cfg.AccessTokenTTL > auth.MaxAccessTokenTTL || cfg.AccessTokenTTL%time.Second != 0 {
		return nil, fmt.Errorf("JWT_ACCESS_TOKEN_TTL must be whole seconds between 1s and 1h")
	}
	return cfg, nil
}
