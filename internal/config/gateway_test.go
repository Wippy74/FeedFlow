package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadGatewayConfig(t *testing.T) {
	for _, name := range []string{"GATEWAY_ADDR", "GATEWAY_API_URL", "GATEWAY_NOTIFICATION_URL", "GATEWAY_TLS_CERT_PATH", "GATEWAY_TLS_KEY_PATH"} {
		t.Setenv(name, "")
	}
	cfg, err := ReadGatewayConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", cfg.Addr)
	require.Equal(t, "http://127.0.0.1:8082", cfg.APIURL)
	require.Equal(t, "http://127.0.0.1:8081", cfg.NotificationURL)
}

func TestReadGatewayConfigRejectsInvalidSettings(t *testing.T) {
	for _, tt := range []struct{ name, value, want string }{
		{"GATEWAY_ADDR", "invalid", "GATEWAY_ADDR"},
		{"GATEWAY_API_URL", "https://example.com:443", "GATEWAY_API_URL"},
		{"GATEWAY_API_URL", "http://example.com:8082/path", "GATEWAY_API_URL"},
		{"GATEWAY_API_URL", "http://user:secret@example.com:8082", "GATEWAY_API_URL"},
		{"GATEWAY_NOTIFICATION_URL", "http://example.com", "GATEWAY_NOTIFICATION_URL"},
		{"GATEWAY_TLS_CERT_PATH", "/tmp/cert.pem", "GATEWAY_TLS_CERT_PATH"},
	} {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			for _, name := range []string{"GATEWAY_ADDR", "GATEWAY_API_URL", "GATEWAY_NOTIFICATION_URL", "GATEWAY_TLS_CERT_PATH", "GATEWAY_TLS_KEY_PATH"} {
				t.Setenv(name, "")
			}
			t.Setenv(tt.name, tt.value)
			_, err := ReadGatewayConfig()
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestReadConfigAPIAddress(t *testing.T) {
	t.Setenv("API_HTTP_ADDR", "")
	cfg, err := ReadConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8082", cfg.HTTPAddr)
	t.Setenv("API_HTTP_ADDR", "localhost:9090")
	cfg, err = ReadConfig()
	require.NoError(t, err)
	require.Equal(t, "localhost:9090", cfg.HTTPAddr)
	t.Setenv("API_HTTP_ADDR", "invalid")
	_, err = ReadConfig()
	require.ErrorContains(t, err, "API_HTTP_ADDR")
}
