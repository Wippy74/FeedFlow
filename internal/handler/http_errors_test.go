package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonolithStorageErrorsDoNotLeakDetails(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	failure := errors.New("SQL with private credentials")
	h := NewHandler(&MockStorage{
		SaveUserFn:     func(context.Context, uuid.UUID, string, string) (model.User, error) { return model.User{}, failure },
		AddFeedFn:      func(context.Context, uuid.UUID, string, string) (model.Feed, error) { return model.Feed{}, failure },
		FollowFeedFn:   func(context.Context, uuid.UUID, uuid.UUID) error { return failure },
		GetPostsFn:     func(context.Context, uuid.UUID, int, int) ([]model.Post, error) { return nil, failure },
		GetFeedsPageFn: func(context.Context, *uuid.UUID, int) ([]model.Feed, error) { return nil, failure },
	}, &MockCache{
		GetPostFn:  func(context.Context, string) ([]model.Post, error) { return nil, redis.Nil },
		GetFeedsFn: func(context.Context, string) ([]model.Feed, error) { return nil, redis.Nil },
	}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	for _, route := range []struct{ method, path, body string }{
		{"POST", "/v1/users", `{"name":"user"}`},
		{"POST", "/v1/feeds", `{"name":"feed","url":"https://example.com/feed"}`},
		{"POST", "/v1/feed_follows", `{"feedId":"` + uuid.NewString() + `"}`},
		{"GET", "/v1/posts", ""}, {"GET", "/v1/feeds", ""},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
		r.Header.Set("Authorization", "Bearer "+token.AccessToken)
		h.InitRouter().ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code, route.path)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "internal_error")
		assert.NotContains(t, w.Body.String(), failure.Error())
	}
}

func TestPostsRejectInvalidPaginationBeforeStorage(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	h := NewHandler(&MockStorage{}, &MockCache{GetPostFn: func(context.Context, string) ([]model.Post, error) {
		t.Fatal("invalid pagination must not query the cache")
		return nil, nil
	}}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	for _, query := range []string{"limit=invalid", "limit=0", "limit=-1", "offset=invalid", "offset=-1"} {
		r := httptest.NewRequest(http.MethodGet, "/v1/posts?"+query, nil)
		r.Header.Set("Authorization", "Bearer "+token.AccessToken)
		w := httptest.NewRecorder()
		h.InitRouter().ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code, query)
	}
}

func TestFollowRejectsClientIdentityInBody(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	h := NewHandler(&MockStorage{FollowFeedFn: func(context.Context, uuid.UUID, uuid.UUID) error {
		t.Fatal("invalid request must not reach storage")
		return nil
	}}, &MockCache{}, WithTokens(issuer, verifier))
	t.Cleanup(func() { require.NoError(t, h.Shutdown(context.Background())) })
	for _, body := range []string{`{"feedId":"` + uuid.NewString() + `","userId":"` + uuid.NewString() + `"}`, `{}`, `{"feedId":"` + uuid.Nil.String() + `"}`} {
		r := httptest.NewRequest(http.MethodPost, "/v1/feed_follows", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token.AccessToken)
		w := httptest.NewRecorder()
		h.InitRouter().ServeHTTP(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
}
