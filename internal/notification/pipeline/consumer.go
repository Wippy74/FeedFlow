package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"FeedFlow/internal/notification/contract"
	"FeedFlow/internal/notification/relay"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

type InboxStore interface {
	AcceptRequest(context.Context, contract.Requested, []byte) error
	ResumeRetry(context.Context, contract.Retry) error
}

type Consumer struct {
	Client    *kgo.Client
	Store     InboxStore
	Publisher relay.Publisher
}

func (c Consumer) Run(ctx context.Context) error {
	if c.Client == nil || c.Store == nil || c.Publisher == nil {
		return fmt.Errorf("notification consumer is not configured")
	}
	for ctx.Err() == nil {
		fetches := c.Client.PollRecords(ctx, 1)
		var fetchErr error
		fetches.EachError(func(_ string, _ int32, err error) { fetchErr = errors.Join(fetchErr, err) })
		if fetchErr != nil {
			c.Client.AllowRebalance()
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("poll Kafka records: %w", fetchErr)
		}
		var processErr error
		fetches.EachRecord(func(record *kgo.Record) {
			if processErr == nil {
				processErr = c.Process(ctx, record)
				if processErr == nil {
					processErr = c.Client.CommitRecords(ctx, record)
				}
			}
		})
		c.Client.AllowRebalance()
		if processErr != nil {
			return fmt.Errorf("process Kafka notification: %w", processErr)
		}
	}
	return nil
}

func (c Consumer) Process(ctx context.Context, record *kgo.Record) error {
	if record == nil {
		return fmt.Errorf("nil Kafka record")
	}
	headers := make(map[string]string, len(record.Headers))
	for _, header := range record.Headers {
		headers[header.Key] = string(header.Value)
	}
	if record.Topic == contract.RequestsTopicV1 {
		event, err := contract.DecodeRequested(record.Key, headers, record.Value)
		if err != nil {
			return c.publishInvalid(ctx, record, "invalid_request")
		}
		if err := c.Store.AcceptRequest(ctx, event, record.Value); err != nil {
			if errors.Is(err, ErrIdentityConflict) {
				return c.publishInvalid(ctx, record, "identity_conflict")
			}
			return err
		}
		return nil
	}
	if !contract.IsRetryTopic(record.Topic) {
		return c.publishInvalid(ctx, record, "invalid_topic")
	}
	if headers["event_type"] != "notification.retry" || headers["schema_version"] != "1" ||
		headers["content-type"] != "application/json" {
		return c.publishInvalid(ctx, record, "invalid_retry")
	}
	if _, err := uuid.Parse(headers["event_id"]); err != nil {
		return c.publishInvalid(ctx, record, "invalid_retry")
	}
	event, err := contract.DecodeRetry(record.Key, record.Value)
	if err != nil {
		return c.publishInvalid(ctx, record, "invalid_retry")
	}
	return c.Store.ResumeRetry(ctx, event)
}

func (c Consumer) publishInvalid(ctx context.Context, source *kgo.Record, reason string) error {
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("%s/%d/%d", source.Topic, source.Partition, source.Offset)))
	payload, _ := json.Marshal(struct {
		SourceTopic string `json:"source_topic"`
		Partition   int32  `json:"partition"`
		Offset      int64  `json:"offset"`
		Reason      string `json:"error_code"`
		OccurredAt  string `json:"occurred_at"`
	}{source.Topic, source.Partition, source.Offset, reason, time.Now().UTC().Format(time.RFC3339Nano)})
	return c.Publisher.Publish(ctx, relay.Record{
		ID: id, Topic: contract.DLQTopicV1, Key: id.String(), Value: payload,
		Headers: map[string]string{"event_id": id.String(), "event_type": "notification.invalid",
			"schema_version": "1", "content-type": "application/json"},
	})
}
