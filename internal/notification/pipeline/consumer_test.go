package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FeedFlow/internal/notification/contract"
	"FeedFlow/internal/notification/relay"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

type inboxStub struct {
	accepted int
	resumed  int
	err      error
}

func (s *inboxStub) AcceptRequest(context.Context, contract.Requested, []byte) error {
	s.accepted++
	return s.err
}
func (s *inboxStub) ResumeRetry(context.Context, contract.Retry) error {
	s.resumed++
	return s.err
}

type publisherStub struct{ records []relay.Record }

func (p *publisherStub) Publish(_ context.Context, record relay.Record) error {
	p.records = append(p.records, record)
	return nil
}

func TestConsumerRoutesInvalidAndConflictingEventsToSafeDLQ(t *testing.T) {
	store := &inboxStub{}
	publisher := &publisherStub{}
	consumer := Consumer{Store: store, Publisher: publisher}
	record := &kgo.Record{Topic: contract.RequestsTopicV1, Partition: 2, Offset: 9,
		Key: []byte("invalid"), Value: []byte(`{"secret":"do-not-copy"}`)}
	require.NoError(t, consumer.Process(context.Background(), record))
	require.Zero(t, store.accepted)
	require.Len(t, publisher.records, 1)
	require.NotContains(t, string(publisher.records[0].Value), "do-not-copy")
	require.Equal(t, "feedflow.notification.dlq.v1", publisher.records[0].Topic)

	id, userID := uuid.New(), uuid.New()
	event := contract.Requested{EventID: id, NotificationID: id, UserID: userID,
		EventType: contract.NotificationRequested, SchemaVersion: 1, Template: contract.TemplateNewPost,
		Producer: "monolith", OccurredAt: time.Now().UTC(), Data: contract.RequestedData{
			PostID: uuid.New(), FeedID: uuid.New(), Title: "post", URL: "https://example.com/post",
			PublishedAt: time.Now().UTC()}}
	record.Key = []byte(userID.String())
	record.Value, _ = json.Marshal(event)
	record.Headers = []kgo.RecordHeader{{Key: "event_id", Value: []byte(id.String())},
		{Key: "event_type", Value: []byte(contract.NotificationRequested)},
		{Key: "schema_version", Value: []byte("1")},
		{Key: "content-type", Value: []byte("application/json")}}
	store.err = ErrIdentityConflict
	require.NoError(t, consumer.Process(context.Background(), record))
	require.Equal(t, 1, store.accepted)
	require.Len(t, publisher.records, 2)
	require.Contains(t, string(publisher.records[1].Value), "identity_conflict")
	store.err = errors.New("database unavailable")
	require.Error(t, consumer.Process(context.Background(), record))
	require.Len(t, publisher.records, 2)

	store.err = nil
	retry := contract.Retry{DeliveryID: uuid.New(), EventID: id, UserID: userID, AttemptNo: 1}
	record.Topic = contract.Retry1mTopicV1
	record.Value, _ = json.Marshal(retry)
	record.Headers[1].Value = []byte("notification.retry")
	require.NoError(t, consumer.Process(context.Background(), record))
	require.Equal(t, 1, store.resumed)
}
