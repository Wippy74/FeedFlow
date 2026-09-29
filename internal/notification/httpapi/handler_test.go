package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationChannelRoutesPassChannelID(t *testing.T) {
	userID := uuid.MustParse("82c52c18-bce8-485e-b322-d7c29c40c250")
	channelID := uuid.MustParse("c40d01de-ec26-44f2-a7a4-ddd5748dc9e9")
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(userID)
	require.NoError(t, err)

	t.Run("patch", func(t *testing.T) {
		called := false
		channels := &MockNotificationChannelStorage{
			SetChannelEnabledFn: func(_ context.Context, gotUserID, gotChannelID uuid.UUID, enabled bool) (notification.Channel, error) {
				called = true
				assert.Equal(t, userID, gotUserID)
				assert.Equal(t, channelID, gotChannelID)
				assert.False(t, enabled)
				return notification.Channel{
					ID:      channelID,
					UserID:  userID,
					Type:    notification.ChannelEmail,
					Enabled: false,
				}, nil
			},
		}
		handler := NewHandler(channels)

		request := httptest.NewRequest(
			http.MethodPatch,
			"/v1/notification-channel/"+channelID.String(),
			strings.NewReader(`{"enabled":false}`),
		)
		request.Header.Set("Authorization", "Bearer "+token.AccessToken)
		response := httptest.NewRecorder()

		handler.Router(verifier).ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.True(t, called)
	})

	t.Run("delete", func(t *testing.T) {
		called := false
		channels := &MockNotificationChannelStorage{
			DeleteChannelFn: func(_ context.Context, gotUserID, gotChannelID uuid.UUID) error {
				called = true
				assert.Equal(t, userID, gotUserID)
				assert.Equal(t, channelID, gotChannelID)
				return nil
			},
		}
		handler := NewHandler(channels)

		request := httptest.NewRequest(
			http.MethodDelete,
			"/v1/notification-channel/"+channelID.String(),
			nil,
		)
		request.Header.Set("Authorization", "Bearer "+token.AccessToken)
		response := httptest.NewRecorder()

		handler.Router(verifier).ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
		assert.True(t, called)
	})
}

func TestChannelCreateAndListUseVerifiedUser(t *testing.T) {
	userID, channelID := uuid.New(), uuid.New()
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(userID)
	require.NoError(t, err)
	channel := notification.Channel{ID: channelID, UserID: userID, Type: notification.ChannelEmail, Destination: "owner@example.com", Enabled: true}
	created, listed := false, false
	store := &MockNotificationChannelStorage{
		CreateChannelFn: func(_ context.Context, got notification.Channel) (notification.Channel, error) {
			created = true
			assert.Equal(t, userID, got.UserID)
			assert.Equal(t, channelID, got.ID)
			assert.Equal(t, channel.Destination, got.Destination)
			return channel, nil
		},
		GetChannelsFn: func(_ context.Context, id uuid.UUID) ([]notification.Channel, error) {
			listed = true
			assert.Equal(t, userID, id)
			return []notification.Channel{channel}, nil
		},
	}
	h := NewHandler(store)
	h.idGenerator = func() uuid.UUID { return channelID }
	router := h.Router(verifier)
	r := httptest.NewRequest(http.MethodPost, "/v1/notification-channel", strings.NewReader(`{"type":"email","destination":" owner@example.com "}`))
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	r.Header.Set("X-User-ID", uuid.NewString())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated, w.Code)
	var got channelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, responseFor(channel), got)
	assert.NotContains(t, w.Body.String(), "user_id")
	assert.True(t, created)
	r = httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil)
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var list []channelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, []channelResponse{responseFor(channel)}, list)
	assert.True(t, listed)
}

func TestEmptyChannelListIsJSONArray(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil)
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	w := httptest.NewRecorder()
	NewHandler(&MockNotificationChannelStorage{}).Router(verifier).ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
}
