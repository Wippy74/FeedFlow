package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddlewareRejectsMalformedHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing header"},
		{name: "scheme only", header: "Bearer"},
		{name: "wrong scheme", header: "ApiKey secret"},
		{name: "too many values", header: "Bearer secret extra"},
		{name: "invalid token", header: "Bearer secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issuer, verifier := testTokens(t)
			storageCalled := false
			cacheCalled := false
			nextCalled := false
			h := NewHandler(&MockStorage{
				GetUserByApiKeyFn: func(context.Context, string) (model.User, error) {
					storageCalled = true
					return model.User{}, nil
				},
			}, &MockCache{
				GetUserFn: func(context.Context, string) (model.User, error) {
					cacheCalled = true
					return model.User{}, nil
				},
			}, &MockNotificationChannelStorage{}, WithTokens(issuer, verifier))
			t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rr := httptest.NewRecorder()

			h.AuthMiddleware(func(http.ResponseWriter, *http.Request) {
				nextCalled = true
			})(rr, req)

			assert.Equal(t, http.StatusUnauthorized, rr.Code)
			assert.False(t, cacheCalled)
			assert.False(t, storageCalled)
			assert.False(t, nextCalled)
		})
	}
}

func TestAuthMiddlewareUsesJWTSubjectWithoutStorageOrCache(t *testing.T) {
	wantUser := model.User{ID: uuid.New()}
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(wantUser.ID)
	require.NoError(t, err)
	h := NewHandler(&MockStorage{
		GetUserByApiKeyFn: func(context.Context, string) (model.User, error) {
			t.Fatal("JWT verification must not query storage")
			return model.User{}, nil
		},
	}, &MockCache{
		GetUserFn: func(context.Context, string) (model.User, error) {
			t.Fatal("JWT verification must not query cache")
			return model.User{}, nil
		},
	}, &MockNotificationChannelStorage{}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "  bearer   "+token.AccessToken+"  ")
	rr := httptest.NewRecorder()

	h.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotUser, ok := r.Context().Value(userContextKey).(model.User)
		require.True(t, ok)
		assert.Equal(t, wantUser, gotUser)
		w.WriteHeader(http.StatusNoContent)
	})(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestAuthMiddlewareRejectsDuplicateHeaders(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	h := NewHandler(&MockStorage{}, &MockCache{}, &MockNotificationChannelStorage{}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	req := httptest.NewRequest(http.MethodGet, "/v1/posts", nil)
	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	req.Header.Add("Authorization", "Bearer "+token.AccessToken)
	rr := httptest.NewRecorder()
	h.InitRouter().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestProtectedRoutesRejectAPIKeys(t *testing.T) {
	issuer, verifier := testTokens(t)
	h := NewHandler(&MockStorage{}, &MockCache{}, &MockNotificationChannelStorage{}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	router := h.InitRouter()
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/feeds"}, {"POST", "/v1/feed_follows"}, {"GET", "/v1/posts"},
		{"POST", "/v1/notification-channel"}, {"GET", "/v1/notification-channel"},
		{"PATCH", "/v1/notification-channel/" + uuid.NewString()}, {"DELETE", "/v1/notification-channel/" + uuid.NewString()},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", "ApiKey key")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code, route.method+" "+route.path)
	}
}

func TestAuthMiddlewareFailsClosedWithoutVerifier(t *testing.T) {
	h := NewHandler(&MockStorage{}, &MockCache{}, &MockNotificationChannelStorage{})
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	req := httptest.NewRequest(http.MethodGet, "/v1/posts", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	h.InitRouter().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}
