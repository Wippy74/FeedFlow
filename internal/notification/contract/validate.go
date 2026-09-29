package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidEvent = errors.New("invalid notification event")

func DecodeRequested(key []byte, headers map[string]string, payload []byte) (Requested, error) {
	var event Requested
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("%w: invalid JSON", ErrInvalidEvent)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return event, fmt.Errorf("%w: trailing JSON", ErrInvalidEvent)
	}
	if event.EventID == uuid.Nil || event.NotificationID != event.EventID || event.UserID == uuid.Nil ||
		event.EventType != NotificationRequested || event.SchemaVersion != RequestedSchemaV1 ||
		event.Template != TemplateNewPost || event.OccurredAt.IsZero() || strings.TrimSpace(event.Producer) == "" ||
		event.Data.PostID == uuid.Nil || event.Data.FeedID == uuid.Nil ||
		strings.TrimSpace(event.Data.Title) == "" || event.Data.PublishedAt.IsZero() ||
		string(key) != event.UserID.String() {
		return event, fmt.Errorf("%w: unsupported or incomplete request", ErrInvalidEvent)
	}
	parsedURL, err := url.Parse(event.Data.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return event, fmt.Errorf("%w: invalid post URL", ErrInvalidEvent)
	}
	for field, want := range map[string]string{
		"event_id": event.EventID.String(), "event_type": NotificationRequested,
		"schema_version": "1", "content-type": "application/json",
	} {
		if headers[field] != want {
			return event, fmt.Errorf("%w: invalid %s header", ErrInvalidEvent, field)
		}
	}
	return event, nil
}

type Retry struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
	EventID    uuid.UUID `json:"event_id"`
	UserID     uuid.UUID `json:"user_id"`
	AttemptNo  int       `json:"attempt_no"`
}

func DecodeRetry(key []byte, payload []byte) (Retry, error) {
	var event Retry
	if err := json.Unmarshal(payload, &event); err != nil || event.DeliveryID == uuid.Nil ||
		event.EventID == uuid.Nil || event.UserID == uuid.Nil || event.AttemptNo <= 0 ||
		string(key) != event.UserID.String() {
		return event, fmt.Errorf("%w: invalid retry", ErrInvalidEvent)
	}
	return event, nil
}
