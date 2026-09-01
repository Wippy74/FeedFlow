package handler

import (
	"NewsAggregator/internal/model"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPostFollowFeedUsesAuthenticatedUser(t *testing.T) {
	authenticatedUser := model.User{ID: uuid.New()}
	requestUserID := uuid.New()
	feedID := uuid.New()

	h := NewHandler(&MockStorage{
		FollowFeedFn: func(_ context.Context, userID, gotFeedID uuid.UUID) error {
			assert.Equal(t, authenticatedUser.ID, userID)
			assert.NotEqual(t, requestUserID, userID)
			assert.Equal(t, feedID, gotFeedID)
			return nil
		},
	}, &MockCache{})

	body := bytes.NewBufferString(`{"userId":"` + requestUserID.String() + `","feedId":"` + feedID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/feed_follows", body)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, authenticatedUser))
	rr := httptest.NewRecorder()

	h.PostFollowFeed(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}
