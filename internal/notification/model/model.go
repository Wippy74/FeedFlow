package notification

import (
	"encoding/json"

	"github.com/google/uuid"
)

type ChannelType string

const (
	ChannelTelegram ChannelType = "telegram"
	ChannelEmail    ChannelType = "email"
)

type DeliveryStatus string

const (
	StatusPending    DeliveryStatus = "pending"
	StatusProcessing DeliveryStatus = "processing"
	StatusSent       DeliveryStatus = "sent"
	StatusFailed     DeliveryStatus = "failed"
)

type Channel struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Type        ChannelType
	Destination string
	Enabled     bool
}
type Event struct {
	ID          uuid.UUID
	Type        string
	AggregateID uuid.UUID
	Payload     json.RawMessage
	Attempts    int
}

type Delivery struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	PostID   uuid.UUID
	Channel  Channel
	Status   DeliveryStatus
	Attempts int
}

type Message struct {
	Recipient string
	Title     string
	Body      string
	URL       string
}
