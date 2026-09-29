package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadNotificationAPIConfig(t *testing.T) {
	t.Setenv("NOTIFICATION_DATABASE_URL", "postgres://notification:secret@localhost/notification")
	t.Setenv("NOTIFICATION_HTTP_ADDR", "")
	t.Setenv("JWT_PUBLIC_KEY_PATH", "/tmp/public.pem")
	t.Setenv("JWT_KEY_ID", "local-1")
	t.Setenv("JWT_ISSUER", "feedflow-auth")
	t.Setenv("JWT_PRIVATE_KEY_PATH", "")
	t.Setenv("DB_HOST", "monolith-host")
	cfg, err := ReadNotificationAPIConfig()
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8081", cfg.HTTPAddr)
	require.Equal(t, "postgres://notification:secret@localhost/notification", cfg.DatabaseURL)
	require.Equal(t, "/tmp/public.pem", cfg.PublicKeyPath)
}

func TestReadNotificationAPIConfigRejectsMissingIndependentSettings(t *testing.T) {
	defaults := map[string]string{
		"NOTIFICATION_DATABASE_URL": "postgres://notification@localhost/notification",
		"NOTIFICATION_HTTP_ADDR":    "127.0.0.1:8081",
		"JWT_PUBLIC_KEY_PATH":       "/tmp/public.pem",
		"JWT_KEY_ID":                "local-1",
		"JWT_ISSUER":                "feedflow-auth",
	}
	for _, tt := range []struct{ key, value, want string }{
		{"NOTIFICATION_DATABASE_URL", "", "NOTIFICATION_DATABASE_URL"},
		{"NOTIFICATION_HTTP_ADDR", "invalid", "NOTIFICATION_HTTP_ADDR"},
		{"JWT_PUBLIC_KEY_PATH", "", "JWT_PUBLIC_KEY_PATH"},
		{"JWT_KEY_ID", "", "JWT_KEY_ID"},
		{"JWT_ISSUER", "", "JWT_ISSUER"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			for key, value := range defaults {
				t.Setenv(key, value)
			}
			t.Setenv(tt.key, tt.value)
			_, err := ReadNotificationAPIConfig()
			require.ErrorContains(t, err, tt.want)
		})
	}
}
