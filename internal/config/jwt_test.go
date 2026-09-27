package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadJWTConfig(t *testing.T) {
	for key, value := range map[string]string{
		"JWT_PRIVATE_KEY_PATH": " private.pem ", "JWT_PUBLIC_KEY_PATH": " public.pem ",
		"JWT_KEY_ID": " key-1 ", "JWT_ISSUER": " feedflow ", "JWT_ACCESS_TOKEN_TTL": "",
	} {
		t.Setenv(key, value)
	}
	cfg, err := ReadJWTConfig()
	require.NoError(t, err)
	assert.Equal(t, "private.pem", cfg.PrivateKeyPath)
	assert.Equal(t, "public.pem", cfg.PublicKeyPath)
	assert.Equal(t, "key-1", cfg.KeyID)
	assert.Equal(t, "feedflow", cfg.Issuer)
	assert.Equal(t, 15*time.Minute, cfg.AccessTokenTTL)
	t.Setenv("JWT_ACCESS_TOKEN_TTL", "5m")
	cfg, err = ReadJWTConfig()
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, cfg.AccessTokenTTL)
	for _, key := range []string{"JWT_PRIVATE_KEY_PATH", "JWT_PUBLIC_KEY_PATH", "JWT_KEY_ID", "JWT_ISSUER"} {
		t.Run("missing "+key, func(t *testing.T) {
			t.Setenv(key, " ")
			_, err := ReadJWTConfig()
			require.Error(t, err)
		})
	}
	for _, value := range []string{"tomorrow", "0s", "-1s", "500ms", "1500ms", "2h"} {
		t.Run("invalid TTL "+value, func(t *testing.T) {
			t.Setenv("JWT_ACCESS_TOKEN_TTL", value)
			_, err := ReadJWTConfig()
			require.Error(t, err)
		})
	}
}
