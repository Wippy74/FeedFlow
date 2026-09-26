package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonolithProtectedRoutesRejectAPIKeys(t *testing.T) {
	issuer, verifier := testTokens(t)
	h := NewHandler(&MockStorage{}, &MockCache{}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/feeds"}, {"POST", "/v1/feed_follows"}, {"GET", "/v1/posts"},
	} {
		r := httptest.NewRequest(route.method, route.path, nil)
		r.Header.Set("Authorization", "ApiKey key")
		w := httptest.NewRecorder()
		h.InitRouter().ServeHTTP(w, r)
		assert.Equal(t, http.StatusUnauthorized, w.Code, route.path)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	}
}

func TestMonolithRouterDoesNotOwnNotificationRoutes(t *testing.T) {
	h := NewHandler(&MockStorage{}, &MockCache{})
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	w := httptest.NewRecorder()
	h.InitRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
