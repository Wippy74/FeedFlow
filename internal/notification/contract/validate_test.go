package contract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDecodeRequestedChecksIdentityAndRouting(t *testing.T) {
	id, userID := uuid.New(), uuid.New()
	event := Requested{EventID: id, NotificationID: id, UserID: userID,
		EventType: NotificationRequested, SchemaVersion: 1, Template: TemplateNewPost,
		Producer: "feedflow-monolith", OccurredAt: time.Now().UTC(),
		Data: RequestedData{PostID: uuid.New(), FeedID: uuid.New(), Title: "post",
			URL: "https://example.com/post", PublishedAt: time.Now().UTC()},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	headers := map[string]string{"event_id": id.String(), "event_type": NotificationRequested,
		"schema_version": "1", "content-type": "application/json"}
	decoded, err := DecodeRequested([]byte(userID.String()), headers, payload)
	require.NoError(t, err)
	require.Equal(t, event.EventID, decoded.EventID)
	_, err = DecodeRequested([]byte(uuid.NewString()), headers, payload)
	require.ErrorIs(t, err, ErrInvalidEvent)
	headers["schema_version"] = "2"
	_, err = DecodeRequested([]byte(userID.String()), headers, payload)
	require.ErrorIs(t, err, ErrInvalidEvent)
	_, err = DecodeRequested([]byte(userID.String()), headers, append(payload, []byte("{}")...))
	require.ErrorIs(t, err, ErrInvalidEvent)
}
