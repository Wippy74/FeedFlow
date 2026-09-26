package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verifierFunc func(string) (uuid.UUID, error)

func (f verifierFunc) Verify(value string) (uuid.UUID, error) { return f(value) }

func TestMiddlewareRejectsMalformedHeaders(t *testing.T) {
	for _, headers := range [][]string{
		nil, {"Bearer"}, {"ApiKey secret"}, {"Bearer secret extra"},
		{"Bearer " + strings.Repeat("x", 8193)}, {"Bearer first", "Bearer second"},
	} {
		t.Run(strings.Join(headers, ",")[:min(40, len(strings.Join(headers, ",")))], func(t *testing.T) {
			called := false
			verifier := verifierFunc(func(string) (uuid.UUID, error) {
				t.Fatal("malformed credentials must not reach the verifier")
				return uuid.Nil, nil
			})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, header := range headers {
				r.Header.Add("Authorization", header)
			}
			w := httptest.NewRecorder()
			Middleware(verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(w, r)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.JSONEq(t, `{"error":{"code":"unauthorized","message":"Bearer access token is required"}}`, w.Body.String())
			assert.Equal(t, `Bearer realm="feedflow"`, w.Header().Get("WWW-Authenticate"))
			assert.False(t, called)
		})
	}
}

func TestMiddlewareUsesVerifiedIdentity(t *testing.T) {
	publicKey, privateKey := testKeys(t)
	issuer, err := NewIssuer(privateKey, "key-1", "feedflow", []string{AudienceAPI, AudienceNotifications}, time.Minute)
	require.NoError(t, err)
	userID := uuid.New()
	token, err := issuer.Issue(userID)
	require.NoError(t, err)
	for _, audience := range []string{AudienceAPI, AudienceNotifications} {
		t.Run(audience, func(t *testing.T) {
			verifier, err := NewVerifier(publicKey, "key-1", "feedflow", audience)
			require.NoError(t, err)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "  bearer   "+token.AccessToken+"  ")
			r.Header.Set("X-User-ID", uuid.NewString())
			w := httptest.NewRecorder()
			Middleware(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, ok := UserIDFromContext(r.Context())
				require.True(t, ok)
				assert.Equal(t, userID, id)
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(w, r)
			assert.Equal(t, http.StatusNoContent, w.Code)
		})
	}
	id, ok := UserIDFromContext(context.Background())
	assert.False(t, ok)
	assert.Equal(t, uuid.Nil, id)
	_, ok = UserIDFromContext(context.WithValue(context.Background(), userIDContextKey{}, uuid.Nil))
	assert.False(t, ok)
}

func TestMiddlewareFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name     string
		verifier TokenVerifier
		status   int
		code     string
	}{
		{"missing verifier", nil, 503, "authentication_unavailable"},
		{"invalid JWT", verifierFunc(func(string) (uuid.UUID, error) { return uuid.Nil, errors.New("verification secret") }), 401, "invalid_token"},
		{"zero subject", verifierFunc(func(string) (uuid.UUID, error) { return uuid.Nil, nil }), 401, "invalid_token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			Middleware(tt.verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("must fail closed") })).ServeHTTP(w, r)
			assert.Equal(t, tt.status, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			assert.Contains(t, w.Body.String(), tt.code)
			assert.NotContains(t, w.Body.String(), "secret")
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}
