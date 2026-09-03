package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfig(t *testing.T) {
	tests := []struct {
		name              string
		envContent        string
		expectedDBUrl     string
		expectedRedisAddr string
		expectedMaxConns  int32
		expectedMinConns  int32
		expectedLifetime  time.Duration
		expectedIdleTime  time.Duration
		expectError       bool
	}{
		{
			name: "Valid environment variables",
			envContent: `
				DB_USER=testuser
				DB_PASSWORD=testpass
				DB_NAME=testdb
				DB_HOST=localhost
				DB_PORT=5432
				REDIS_HOST=localhost
				REDIS_PORT=6379
				DB_MAX_CONNS=20
				DB_MIN_CONNS=4
				DB_MAX_CONN_LIFETIME=2h
				DB_MAX_CONN_IDLE_TIME=15m
				`,
			expectedDBUrl:     "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable",
			expectedRedisAddr: "localhost:6379",
			expectedMaxConns:  20,
			expectedMinConns:  4,
			expectedLifetime:  2 * time.Hour,
			expectedIdleTime:  15 * time.Minute,
			expectError:       false,
		},
		{
			name: "Missing variables (should still create struct, though fields might be empty)",
			envContent: `
				DB_USER=testuser
				DB_HOST=localhost
				REDIS_HOST=localhost
				`,
			expectedDBUrl:     "postgres://testuser:@localhost:/?sslmode=disable",
			expectedRedisAddr: "localhost:",
			expectedMaxConns:  defaultDBMaxConns,
			expectedMinConns:  defaultDBMinConns,
			expectedLifetime:  defaultDBMaxConnLifetime,
			expectedIdleTime:  defaultDBMaxConnIdleTime,
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := []string{
				"DB_USER", "DB_PASSWORD", "DB_NAME", "DB_HOST", "DB_PORT",
				"REDIS_HOST", "REDIS_PORT", "DB_MAX_CONNS", "DB_MIN_CONNS",
				"DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME",
			}
			for _, k := range envVars {
				_ = os.Unsetenv(k)
			}

			err := os.WriteFile(".env", []byte(tt.envContent), 0644)
			require.NoError(t, err)
			defer func() { _ = os.Remove(".env") }()

			cfg, err := ReadConfig()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				assert.Equal(t, tt.expectedDBUrl, cfg.DBUrl)
				assert.Equal(t, tt.expectedRedisAddr, cfg.RedisAddr)
				assert.Equal(t, tt.expectedMaxConns, cfg.DBMaxConns)
				assert.Equal(t, tt.expectedMinConns, cfg.DBMinConns)
				assert.Equal(t, tt.expectedLifetime, cfg.DBMaxConnLifetime)
				assert.Equal(t, tt.expectedIdleTime, cfg.DBMaxConnIdleTime)
			}
		})
	}

	invalidPoolConfigs := []struct {
		name       string
		poolConfig string
	}{
		{name: "invalid maximum", poolConfig: "DB_MAX_CONNS=not-a-number"},
		{name: "zero maximum", poolConfig: "DB_MAX_CONNS=0"},
		{name: "minimum exceeds maximum", poolConfig: "DB_MAX_CONNS=2\nDB_MIN_CONNS=3"},
		{name: "invalid lifetime", poolConfig: "DB_MAX_CONN_LIFETIME=tomorrow"},
		{name: "zero idle time", poolConfig: "DB_MAX_CONN_IDLE_TIME=0s"},
	}
	for _, tt := range invalidPoolConfigs {
		t.Run(tt.name, func(t *testing.T) {
			envVars := []string{"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME"}
			for _, key := range envVars {
				_ = os.Unsetenv(key)
			}

			err := os.WriteFile(".env", []byte(tt.poolConfig), 0644)
			require.NoError(t, err)
			defer func() { _ = os.Remove(".env") }()

			cfg, err := ReadConfig()
			require.Error(t, err)
			assert.Nil(t, cfg)
		})
	}

	t.Run("Missing .env file", func(t *testing.T) {
		_ = os.Remove(".env")
		cfg, err := ReadConfig()
		require.Error(t, err)
		require.Nil(t, cfg)
	})
}
