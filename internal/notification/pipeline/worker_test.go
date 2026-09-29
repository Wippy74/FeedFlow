package pipeline

import (
	"context"
	"testing"
	"time"

	notification "FeedFlow/internal/notification/model"
	"FeedFlow/internal/notification/retry"
	sender "FeedFlow/internal/notification/sender"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type deliveryStoreStub struct {
	sent      int
	failures  int
	delay     time.Duration
	errorCode string
}

func (*deliveryStoreStub) Claim(context.Context, int, time.Duration, []string) ([]Task, error) {
	return nil, nil
}
func (s *deliveryStoreStub) MarkSent(context.Context, Task) error { s.sent++; return nil }
func (s *deliveryStoreStub) MarkFailed(_ context.Context, _ Task, delay time.Duration, code string) error {
	s.failures++
	s.delay = delay
	s.errorCode = code
	return nil
}
func (*deliveryStoreStub) RecoverExpired(context.Context, int, int) (int, error) { return 0, nil }

type senderStub struct{ err error }

func (s senderStub) Send(context.Context, notification.Message) error { return s.err }

type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary secret" }
func (temporaryError) Retryable() bool { return true }

func TestWorkerSchedulesRetryAndTerminalFailure(t *testing.T) {
	store := &deliveryStoreStub{}
	policy, err := retry.NewPolicy(retry.Config{MaxAttempts: 2, BaseDelay: time.Second,
		MaxDelay: time.Minute, RandomFloat64: func() float64 { return 0.5 }})
	require.NoError(t, err)
	worker := Worker{Store: store, Senders: sender.Registry{
		notification.ChannelEmail: senderStub{err: temporaryError{}},
	}, Policy: policy}
	task := Task{ID: uuid.New(), Attempts: 1, ChannelType: notification.ChannelEmail}
	require.NoError(t, worker.Process(context.Background(), task))
	require.Equal(t, 1, store.failures)
	require.Positive(t, store.delay)
	require.Equal(t, "provider_error", store.errorCode)
	task.Attempts = 2
	require.NoError(t, worker.Process(context.Background(), task))
	require.Equal(t, 2, store.failures)
	require.Zero(t, store.delay)
	worker.Senders[notification.ChannelEmail] = senderStub{err: nil}
	require.NoError(t, worker.Process(context.Background(), task))
	require.Equal(t, 1, store.sent)
}
