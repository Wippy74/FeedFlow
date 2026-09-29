package broker

import (
	"context"
	"fmt"
	"strings"

	"FeedFlow/internal/notification/relay"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Publisher struct{ Client *kgo.Client }

func NewPublisher(brokers string) (*Publisher, error) {
	addresses := splitBrokers(brokers)
	if len(addresses) == 0 {
		return nil, fmt.Errorf("KAFKA_BROKERS must not be empty")
	}
	client, err := kgo.NewClient(kgo.SeedBrokers(addresses...), kgo.RequiredAcks(kgo.AllISRAcks()))
	if err != nil {
		return nil, fmt.Errorf("create Kafka publisher: %w", err)
	}
	return &Publisher{Client: client}, nil
}

func splitBrokers(value string) []string {
	var addresses []string
	for _, address := range strings.Split(value, ",") {
		if address = strings.TrimSpace(address); address != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func (p *Publisher) Publish(ctx context.Context, value relay.Record) error {
	if p == nil || p.Client == nil {
		return fmt.Errorf("kafka publisher is unavailable")
	}
	headers := make([]kgo.RecordHeader, 0, len(value.Headers))
	for _, key := range []string{"event_id", "event_type", "schema_version", "content-type"} {
		if header, ok := value.Headers[key]; ok {
			headers = append(headers, kgo.RecordHeader{Key: key, Value: []byte(header)})
		}
	}
	_, err := p.Client.ProduceSync(ctx, &kgo.Record{
		Topic: value.Topic, Key: []byte(value.Key), Value: value.Value, Headers: headers,
	}).First()
	return err
}

func (p *Publisher) Close() { p.Client.Close() }

func NewConsumer(brokers, group string, topics ...string) (*kgo.Client, error) {
	addresses := splitBrokers(brokers)
	if len(addresses) == 0 || group == "" || len(topics) == 0 {
		return nil, fmt.Errorf("kafka consumer requires brokers, group and topics")
	}
	return kgo.NewClient(kgo.SeedBrokers(addresses...), kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll())
}
