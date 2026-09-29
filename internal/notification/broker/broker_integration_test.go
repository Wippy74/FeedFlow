package broker

import (
	"context"
	"os"
	"testing"
	"time"

	"FeedFlow/internal/notification/relay"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKafkaIntegrationPublishAndCommit(t *testing.T) {
	brokers, topic := os.Getenv("FEEDFLOW_TEST_KAFKA_BROKERS"), os.Getenv("FEEDFLOW_TEST_KAFKA_TOPIC")
	if brokers == "" || topic == "" {
		t.Skip("Kafka integration environment is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	publisher, err := NewPublisher(brokers)
	require.NoError(t, err)
	defer publisher.Close()
	consumer, err := NewConsumer(brokers, "feedflow-test-"+uuid.NewString(), topic)
	require.NoError(t, err)
	defer consumer.Close()
	id := uuid.New()
	require.NoError(t, publisher.Publish(ctx, relay.Record{
		ID: id, Topic: topic, Key: id.String(), Value: []byte(`{"test":true}`),
		Headers: map[string]string{"event_id": id.String(), "event_type": "notification.test",
			"schema_version": "1", "content-type": "application/json"},
	}))
	for ctx.Err() == nil {
		fetches := consumer.PollRecords(ctx, 1)
		var received *kgo.Record
		fetches.EachRecord(func(record *kgo.Record) { received = record })
		if received != nil {
			require.Equal(t, id.String(), string(received.Key))
			require.JSONEq(t, `{"test":true}`, string(received.Value))
			require.NoError(t, consumer.CommitRecords(ctx, received))
			consumer.AllowRebalance()
			return
		}
		consumer.AllowRebalance()
	}
	require.Fail(t, "timed out waiting for Kafka record")
}
