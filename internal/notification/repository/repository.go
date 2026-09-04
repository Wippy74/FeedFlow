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

	ClaimDeliveries(ctx context.Context, limit int) ([]notification.Delivery, error)
	MarkDeliverySent(ctx context.Context, id uuid.UUID) error
	ScheduleDeliveryRetry(ctx context.Context, id uuid.UUID, availableAt time.Time, reason string) error
	MarkDeliveryFailed(ctx context.Context, id uuid.UUID, reason string) error
}
