package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsJWTMisconfigurationBeforeConnectingToDatabase(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	wrongPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyDir := t.TempDir()
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	wrongPublicDER, err := x509.MarshalPKIXPublicKey(wrongPublicKey)
	require.NoError(t, err)
	privatePath := filepath.Join(keyDir, "private.pem")
	publicPath := filepath.Join(keyDir, "public.pem")
	wrongPublicPath := filepath.Join(keyDir, "wrong-public.pem")
	require.NoError(t, os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0600))
	require.NoError(t, os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0600))
	require.NoError(t, os.WriteFile(wrongPublicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: wrongPublicDER}), 0600))
	for _, tt := range []struct{ name, privatePath, publicPath, want string }{
		{"missing configuration", "", "", "JWT_PRIVATE_KEY_PATH"},
		{"missing key file", filepath.Join(keyDir, "missing.pem"), publicPath, "read JWT private key"},
		{"mismatched keys", privatePath, wrongPublicPath, "does not match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_PRIVATE_KEY_PATH", tt.privatePath)
			t.Setenv("JWT_PUBLIC_KEY_PATH", tt.publicPath)
			t.Setenv("JWT_KEY_ID", "test-1")
			t.Setenv("JWT_ISSUER", "feedflow")
			t.Setenv("JWT_ACCESS_TOKEN_TTL", "15m")
			err := run()
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestMonolithRouterDoesNotServeNotificationChannels(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	apiVerifier, err := auth.NewVerifier(publicKey, "key-1", "feedflow", auth.AudienceAPI)
	require.NoError(t, err)
	monolith := handler.NewHandler(nil, nil, handler.WithTokens(nil, apiVerifier))
	t.Cleanup(func() { require.NoError(t, monolith.Shutdown(context.Background())) })
	router := monolith.InitRouter()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		r := httptest.NewRequest(method, "/v1/notification-channel", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
}
