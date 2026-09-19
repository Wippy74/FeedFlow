package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"FeedFlow/internal/model"
	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationChannelRoutesPassChannelID(t *testing.T) {
	userID := uuid.MustParse("82c52c18-bce8-485e-b322-d7c29c40c250")
	channelID := uuid.MustParse("c40d01de-ec26-44f2-a7a4-ddd5748dc9e9")
	user := model.User{ID: userID}

	cache := &MockCache{GetUserFn: func(context.Context, string) (model.User, error) {
		return user, nil
	}}

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
		handler := NewHandler(&MockStorage{}, cache, channels)
		t.Cleanup(func() { require.NoError(t, handler.Shutdown(context.Background())) })

		request := httptest.NewRequest(
			http.MethodPatch,
			"/v1/notification-channel/"+channelID.String(),
			strings.NewReader(`{"enabled":false}`),
		)
		request.Header.Set("Authorization", "ApiKey test-key")
		response := httptest.NewRecorder()

		handler.InitRouter().ServeHTTP(response, request)

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
		handler := NewHandler(&MockStorage{}, cache, channels)
		t.Cleanup(func() { require.NoError(t, handler.Shutdown(context.Background())) })

		request := httptest.NewRequest(
			http.MethodDelete,
			"/v1/notification-channel/"+channelID.String(),
			nil,
		)
		request.Header.Set("Authorization", "ApiKey test-key")
		response := httptest.NewRecorder()

		handler.InitRouter().ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
		assert.True(t, called)
	})
}
