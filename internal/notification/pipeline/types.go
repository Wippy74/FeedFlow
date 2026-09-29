package pipeline

import (
	"context"
	"errors"
	"time"

	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
)

var ErrIdentityConflict = errors.New("notification event ID conflicts with stored payload")

type Task struct {
	ID          uuid.UUID
	EventID     uuid.UUID
	UserID      uuid.UUID
	ClaimToken  uuid.UUID
	Attempts    int
	ChannelType notification.ChannelType
	Message     notification.Message
}

type Store interface {
	Claim(context.Context, int, time.Duration, []string) ([]Task, error)
	MarkSent(context.Context, Task) error
	MarkFailed(context.Context, Task, time.Duration, string) error
	RecoverExpired(context.Context, int, int) (int, error)
}
