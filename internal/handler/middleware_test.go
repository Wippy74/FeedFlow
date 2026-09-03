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
		{name: "scheme only", header: "ApiKey"},
		{name: "wrong scheme", header: "Bearer secret"},
		{name: "too many values", header: "ApiKey secret extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			})

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

func TestAuthMiddlewareUsesCachedUser(t *testing.T) {
	wantUser := model.User{ID: uuid.New(), Name: "cached user"}
	h := NewHandler(&MockStorage{
		GetUserByApiKeyFn: func(context.Context, string) (model.User, error) {
			t.Fatal("storage must not be called on cache hit")
			return model.User{}, nil
		},
	}, &MockCache{
		GetUserFn: func(_ context.Context, key string) (model.User, error) {
			assert.Equal(t, "auth:apikey:secret", key)
			return wantUser, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "  ApiKey   secret  ")
	rr := httptest.NewRecorder()

	h.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		gotUser, ok := r.Context().Value(userContextKey).(model.User)
		require.True(t, ok)
		assert.Equal(t, wantUser, gotUser)
		w.WriteHeader(http.StatusNoContent)
	})(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}
