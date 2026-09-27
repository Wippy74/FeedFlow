package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"FeedFlow/internal/auth"
	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testTokens(t *testing.T) (*auth.Issuer, *auth.Verifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	issuer, err := auth.NewIssuer(privateKey, "key-1", "feedflow", []string{auth.AudienceAPI, auth.AudienceNotifications}, 15*time.Minute)
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(publicKey, "key-1", "feedflow", auth.AudienceNotifications)
	require.NoError(t, err)
	return issuer, verifier
}

type MockNotificationChannelStorage struct {
	CreateChannelFn     func(ctx context.Context, channel notification.Channel) (notification.Channel, error)
	GetChannelsFn       func(ctx context.Context, userID uuid.UUID) ([]notification.Channel, error)
	SetChannelEnabledFn func(ctx context.Context, userID, channelID uuid.UUID, enabled bool) (notification.Channel, error)
	DeleteChannelFn     func(ctx context.Context, userID, channelID uuid.UUID) error
}

func (m *MockNotificationChannelStorage) CreateChannel(ctx context.Context, channel notification.Channel) (notification.Channel, error) {
	if m.CreateChannelFn != nil {
		return m.CreateChannelFn(ctx, channel)
	}
	return notification.Channel{}, nil
}

func (m *MockNotificationChannelStorage) GetChannels(ctx context.Context, userID uuid.UUID) ([]notification.Channel, error) {
	if m.GetChannelsFn != nil {
		return m.GetChannelsFn(ctx, userID)
	}
	return []notification.Channel{}, nil
}

func (m *MockNotificationChannelStorage) SetChannelEnabled(ctx context.Context, userID uuid.UUID, channelID uuid.UUID, enabled bool) (notification.Channel, error) {
	if m.SetChannelEnabledFn != nil {
		return m.SetChannelEnabledFn(ctx, userID, channelID, enabled)
	}
	return notification.Channel{}, nil
}

func (m *MockNotificationChannelStorage) DeleteChannel(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) error {
	if m.DeleteChannelFn != nil {
		return m.DeleteChannelFn(ctx, userID, channelID)
	}
	return nil
}
