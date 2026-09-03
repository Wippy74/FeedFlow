package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPostsReturnsImmediatelyOnCacheHit(t *testing.T) {
	user := model.User{ID: uuid.New()}
	wantPosts := []model.Post{{ID: uuid.New(), Title: "cached post"}}
	h := NewHandler(&MockStorage{
		GetPostsFn: func(context.Context, uuid.UUID, int, int) ([]model.Post, error) {
			t.Fatal("storage must not be called on cache hit")
			return nil, nil
		},
	}, &MockCache{
		GetPostFn: func(_ context.Context, key string) ([]model.Post, error) {
			assert.Equal(t, "posts:user:"+user.ID.String()+":limit:10:offset:0", key)
			return wantPosts, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/posts", nil)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, user))
	rr := httptest.NewRecorder()

	h.GetPosts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "HIT", rr.Header().Get("X-Cache"))
	decoder := json.NewDecoder(rr.Body)
	var gotPosts []model.Post
	require.NoError(t, decoder.Decode(&gotPosts))
	assert.Equal(t, wantPosts, gotPosts)
	var extra any
	assert.ErrorIs(t, decoder.Decode(&extra), io.EOF, "response must contain only one JSON value")
}
