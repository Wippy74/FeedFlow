package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPostFeedGeneratesIDOnServer(t *testing.T) {
	serverID := uuid.New()
	cacheDeleted := false
	h := NewHandler(&MockStorage{
		AddFeedFn: func(_ context.Context, id uuid.UUID, name, url string) (model.Feed, error) {
			assert.Equal(t, serverID, id)
			assert.Equal(t, "Example", name)
			assert.Equal(t, "https://example.com/rss", url)
			return model.Feed{ID: id, Name: name, Url: url}, nil
		},
	}, &MockCache{
		DeleteFn: func(_ context.Context, key string) error {
			assert.Equal(t, "feeds:all", key)
			cacheDeleted = true
			return nil
		},
	})
	h.idGenerator = func() uuid.UUID { return serverID }

	body := bytes.NewBufferString(`{"name":"Example","url":"https://example.com/rss"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds", body)
	rr := httptest.NewRecorder()

	h.PostFeed(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, cacheDeleted, "cache must be invalidated before the handler returns")
}

func TestPostFeedRejectsClientGeneratedID(t *testing.T) {
	h := NewHandler(&MockStorage{
		AddFeedFn: func(context.Context, uuid.UUID, string, string) (model.Feed, error) {
			t.Fatal("storage must not be called when request contains a server-managed ID")
			return model.Feed{}, nil
		},
	}, &MockCache{})

	body := bytes.NewBufferString(`{"id":"` + uuid.New().String() + `","name":"Example","url":"https://example.com/rss"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds", body)
	rr := httptest.NewRecorder()

	h.PostFeed(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
