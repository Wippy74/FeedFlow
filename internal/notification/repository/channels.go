package notification

import (
	"context"
	"errors"

	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
)

var (
	ErrChannelNotFound = errors.New("notification channel not found")
	ErrChannelConflict = errors.New("notification channel already exists")
	ErrUnavailable     = errors.New("notification storage unavailable")
)

type ChannelRepository interface {
	CreateChannel(context.Context, notification.Channel) (notification.Channel, error)
	GetChannels(context.Context, uuid.UUID) ([]notification.Channel, error)
	SetChannelEnabled(context.Context, uuid.UUID, uuid.UUID, bool) (notification.Channel, error)
	DeleteChannel(context.Context, uuid.UUID, uuid.UUID) error
}
