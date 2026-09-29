package contract

import (
	"time"

	"github.com/google/uuid"
)

const (
	RequestsTopicV1       = "feedflow.notification.requests.v1"
	Retry1mTopicV1        = "feedflow.notification.retry.1m.v1"
	Retry10mTopicV1       = "feedflow.notification.retry.10m.v1"
	Retry1hTopicV1        = "feedflow.notification.retry.1h.v1"
	DLQTopicV1            = "feedflow.notification.dlq.v1"
	NotificationRequested = "notification.requested"
	RequestedSchemaV1     = 1
	TemplateNewPost       = "new_post"
)

func IsRetryTopic(topic string) bool {
	return topic == Retry1mTopicV1 || topic == Retry10mTopicV1 || topic == Retry1hTopicV1
}

type Requested struct {
	EventID        uuid.UUID     `json:"event_id"`
	EventType      string        `json:"event_type"`
	SchemaVersion  int           `json:"schema_version"`
	OccurredAt     time.Time     `json:"occurred_at"`
	Producer       string        `json:"producer"`
	NotificationID uuid.UUID     `json:"notification_id"`
	UserID         uuid.UUID     `json:"user_id"`
	Template       string        `json:"template"`
	Data           RequestedData `json:"data"`
}

type RequestedData struct {
	PostID      uuid.UUID `json:"post_id"`
	FeedID      uuid.UUID `json:"feed_id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}
