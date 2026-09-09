package notification

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	GetPendingEvents(ctx context.Context, limit int) ([]notification.Event, error)
	CreateDeliveries(ctx context.Context, event notification.Event) error
	MarkEventProcessed(ctx context.Context, eventID uuid.UUID) error

	CreateChannel(ctx context.Context, channel notification.Channel) (notification.Channel, error)
	GetUserChannels(ctx context.Context, userID uuid.UUID) ([]notification.Channel, error)
	SetChannelEnabled(ctx context.Context, userID uuid.UUID, channelID uuid.UUID, enabled bool) (notification.Channel, error)
	DeleteChannel(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) error
	ExpandPostCreatedEvents(ctx context.Context, limit int) (int64, error)
	ClaimDeliveries(ctx context.Context, limit int) ([]notification.DeliveryTask, error)
	MarkDeliverySent(ctx context.Context, id uuid.UUID) error
	ScheduleDeliveryRetry(ctx context.Context, id uuid.UUID, availableAt time.Time, reason string) error
	MarkDeliveryFailed(ctx context.Context, id uuid.UUID, reason string) error
	RequeueStaleDeliveries(ctx context.Context, staleBefore time.Time) (int64, error)
}
