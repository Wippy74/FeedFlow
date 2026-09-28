package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type GatewayConfig struct {
	Addr            string
	APIURL          string
	NotificationURL string
	TLSCertPath     string
	TLSKeyPath      string
}

func ReadGatewayConfig() (*GatewayConfig, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env: %w", err)
	}
	cfg := &GatewayConfig{
		Addr:            strings.TrimSpace(os.Getenv("GATEWAY_ADDR")),
		APIURL:          strings.TrimSpace(os.Getenv("GATEWAY_API_URL")),
		NotificationURL: strings.TrimSpace(os.Getenv("GATEWAY_NOTIFICATION_URL")),
		TLSCertPath:     strings.TrimSpace(os.Getenv("GATEWAY_TLS_CERT_PATH")),
		TLSKeyPath:      strings.TrimSpace(os.Getenv("GATEWAY_TLS_KEY_PATH")),
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "http://127.0.0.1:8082"
	}
	if cfg.NotificationURL == "" {
		cfg.NotificationURL = "http://127.0.0.1:8081"
	}
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return nil, fmt.Errorf("invalid GATEWAY_ADDR: %w", err)
	}
	for _, upstream := range []struct{ name, value string }{
		{"GATEWAY_API_URL", cfg.APIURL},
		{"GATEWAY_NOTIFICATION_URL", cfg.NotificationURL},
	} {
		parsed, err := url.Parse(upstream.value)
		if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.Port() == "" ||
			parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
			return nil, fmt.Errorf("%s must be an HTTP origin with host and port", upstream.name)
		}
	}
	if (cfg.TLSCertPath == "") != (cfg.TLSKeyPath == "") {
		return nil, fmt.Errorf("GATEWAY_TLS_CERT_PATH and GATEWAY_TLS_KEY_PATH must be set together")
	}
	return cfg, nil
}
