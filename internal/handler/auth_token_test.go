package handler

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTokens(t *testing.T) (*auth.Issuer, *auth.Verifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	issuer, err := auth.NewIssuer(privateKey, "key-1", "feedflow", []string{auth.AudienceAPI, auth.AudienceNotifications}, 15*time.Minute)
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(publicKey, "key-1", "feedflow", auth.AudienceAPI)
	require.NoError(t, err)
	return issuer, verifier
}

type tokenIssuerFunc func(uuid.UUID) (auth.AccessToken, error)

func (f tokenIssuerFunc) Issue(id uuid.UUID) (auth.AccessToken, error) { return f(id) }

func TestTokenExchangeAndProtectedRoute(t *testing.T) {
	issuer, verifier := testTokens(t)
	userID := uuid.New()
	lookups := 0
	h := NewHandler(&MockStorage{
		GetPostsFn: func(_ context.Context, id uuid.UUID, _, _ int) ([]model.Post, error) {
			assert.Equal(t, userID, id)
			return nil, nil
		},
		GetUserByApiKeyFn: func(_ context.Context, value string) (model.User, error) {
			lookups++
			assert.Equal(t, "test-key", value)
			return model.User{ID: userID, Name: "test user", ApiKey: "test-key"}, nil
		},
	}, &MockCache{
		GetPostFn: func(context.Context, string) ([]model.Post, error) { return nil, redis.Nil },
		GetUserFn: func(context.Context, string) (model.User, error) {
			t.Fatal("token authentication must not use the credential cache")
			return model.User{}, nil
		},
	}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	router := h.InitRouter()
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	request.Header.Set("Authorization", "ApiKey test-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", response.Header().Get("Pragma"))
	var token auth.AccessToken
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &token))
	assert.Equal(t, int64(900), token.ExpiresIn)
	assert.Equal(t, "Bearer", token.TokenType)
	assert.NotContains(t, response.Body.String(), "test-key")
	gotID, err := verifier.Verify(token.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, userID, gotID)
	request = httptest.NewRequest(http.MethodGet, "/v1/posts", nil)
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, 1, lookups, "business request must not perform another credential lookup")
}

func TestTokenExchangeErrors(t *testing.T) {
	issuer, _ := testTokens(t)
	tests := []struct {
		name    string
		header  string
		user    model.User
		err     error
		issuer  TokenIssuer
		status  int
		lookups int
	}{
		{name: "missing header", issuer: issuer, status: 401},
		{name: "Bearer is not a credential", header: "Bearer token", issuer: issuer, status: 401},
		{name: "too many header values", header: "ApiKey key extra", issuer: issuer, status: 401},
		{name: "invalid key", header: "ApiKey key", err: pgx.ErrNoRows, issuer: issuer, status: 401, lookups: 1},
		{name: "zero user", header: "ApiKey key", issuer: issuer, status: 401, lookups: 1},
		{name: "database unavailable", header: "ApiKey key", err: errors.New("database secret"), issuer: issuer, status: 503, lookups: 1},
		{name: "issuer unavailable", header: "ApiKey key", status: 503},
		{name: "signing error", header: "ApiKey key", user: model.User{ID: uuid.New()}, status: 500, lookups: 1,
			issuer: tokenIssuerFunc(func(uuid.UUID) (auth.AccessToken, error) { return auth.AccessToken{}, errors.New("signing secret") })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookups := 0
			h := NewHandler(&MockStorage{GetUserByApiKeyFn: func(context.Context, string) (model.User, error) {
				lookups++
				return tt.user, tt.err
			}}, &MockCache{}, WithTokens(tt.issuer, nil))
			t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
			request := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			response := httptest.NewRecorder()
			h.InitRouter().ServeHTTP(response, request)
			assert.Equal(t, tt.status, response.Code)
			assert.Equal(t, tt.lookups, lookups)
			assert.NotContains(t, response.Body.String(), "secret")
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			var payload map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
			assert.Contains(t, payload, "error")
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		})
	}
}
