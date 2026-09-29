package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FeedFlow/internal/auth"
	notificationhttp "FeedFlow/internal/notification/httpapi"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestNotificationAPIRoutesRequireNotificationAudience(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(publicKey, "key-1", "feedflow", auth.AudienceNotifications)
	require.NoError(t, err)
	router := notificationhttp.NewHandler(nil).Router(verifier)
	for _, tt := range []struct {
		name      string
		audiences []string
		status    int
	}{
		{"monolith audience", []string{auth.AudienceAPI}, http.StatusUnauthorized},
		{"notification audience", []string{auth.AudienceNotifications}, http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			issuer, err := auth.NewIssuer(privateKey, "key-1", "feedflow", tt.audiences, time.Minute)
			require.NoError(t, err)
			token, err := issuer.Issue(uuid.New())
			require.NoError(t, err)
			r := httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil)
			r.Header.Set("Authorization", "Bearer "+token.AccessToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, tt.status, w.Code)
		})
	}
	for _, path := range []string{"/v1/feeds", "/v1/auth/token", "/v1/users"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, w.Code)
	}
}

func TestNotificationAPIStartsWithChannelsTableOnly(t *testing.T) {
	databaseURL := os.Getenv("FEEDFLOW_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("FEEDFLOW_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"notification_api_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, dropErr)
	})
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err)
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err)
	defer pool.Close()
	migration, err := os.ReadFile(filepath.Join("..", "..", "internal", "notification", "migrations", "sql", "001_notification_channels.up.sql"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(migration), pgx.QueryExecModeSimpleProtocol)
	require.NoError(t, err)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	publicPath := filepath.Join(t.TempDir(), "public.pem")
	require.NoError(t, os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0600))
	serviceURL, err := url.Parse(databaseURL)
	require.NoError(t, err)
	params := serviceURL.Query()
	params.Set("search_path", schema)
	serviceURL.RawQuery = params.Encode()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	t.Setenv("NOTIFICATION_DATABASE_URL", serviceURL.String())
	t.Setenv("NOTIFICATION_HTTP_ADDR", address)
	t.Setenv("JWT_PUBLIC_KEY_PATH", publicPath)
	t.Setenv("JWT_PRIVATE_KEY_PATH", "")
	t.Setenv("JWT_KEY_ID", "test-key")
	t.Setenv("JWT_ISSUER", "feedflow-test")
	serviceCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	done := make(chan error, 1)
	go func() { done <- run(serviceCtx) }()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	require.Eventually(t, func() bool {
		response, requestErr := client.Get("http://" + address + "/v1/notification-channel")
		if requestErr != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode == http.StatusUnauthorized
	}, 5*time.Second, 20*time.Millisecond)
	issuer, err := auth.NewIssuer(privateKey, "test-key", "feedflow-test", []string{auth.AudienceNotifications}, time.Minute)
	require.NoError(t, err)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodGet, "http://"+address+"/v1/notification-channel", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err := client.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	stop()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("notification API did not stop after context cancellation")
	}
}
