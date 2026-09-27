package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	body := bytes.NewBufferString(`{"feedId":"` + feedID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/feed_follows", body)
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(authenticatedUser.ID)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-User-ID", requestUserID.String())
	rr := httptest.NewRecorder()

	auth.Middleware(verifier)(http.HandlerFunc(h.PostFollowFeed)).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}
