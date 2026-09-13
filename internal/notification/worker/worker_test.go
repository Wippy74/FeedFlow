package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	notification "FeedFlow/internal/notification/model"
	sender "FeedFlow/internal/notification/sender"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repositoryMock struct {
	expandFn   func(context.Context, int) (int64, error)
	claimFn    func(context.Context, int) ([]notification.DeliveryTask, error)
	markSentFn func(context.Context, uuid.UUID) error
	retryFn    func(context.Context, uuid.UUID, time.Time, string) error
	failedFn   func(context.Context, uuid.UUID, string) error
	requeueFn  func(context.Context, time.Time) (int64, error)
}

func (m *repositoryMock) ExpandPostCreatedEvents(ctx context.Context, limit int) (int64, error) {
	if m.expandFn != nil {
		return m.expandFn(ctx, limit)
	}
	return 0, nil
}

func (m *repositoryMock) ClaimDeliveries(ctx context.Context, limit int) ([]notification.DeliveryTask, error) {
	if m.claimFn != nil {
		return m.claimFn(ctx, limit)
	}
	return nil, nil
}

func (m *repositoryMock) MarkDeliverySent(ctx context.Context, id uuid.UUID) error {
	if m.markSentFn != nil {
		return m.markSentFn(ctx, id)
	}
	return nil
}

func (m *repositoryMock) ScheduleDeliveryRetry(
	ctx context.Context,
	id uuid.UUID,
	availableAt time.Time,
	reason string,
) error {
	if m.retryFn != nil {
		return m.retryFn(ctx, id, availableAt, reason)
	}
	return nil
}

func (m *repositoryMock) MarkDeliveryFailed(ctx context.Context, id uuid.UUID, reason string) error {
	if m.failedFn != nil {
		return m.failedFn(ctx, id, reason)
	}
	return nil
}

func (m *repositoryMock) RequeueStaleDeliveries(ctx context.Context, staleBefore time.Time) (int64, error) {
	if m.requeueFn != nil {
		return m.requeueFn(ctx, staleBefore)
	}
	return 0, nil
}

type senderRegistryMock struct {
	getFn func(notification.ChannelType) (sender.Sender, error)
}

func (m *senderRegistryMock) Get(channelType notification.ChannelType) (sender.Sender, error) {
	return m.getFn(channelType)
}

type senderFunc func(context.Context, notification.Message) error

func (fn senderFunc) Send(ctx context.Context, message notification.Message) error {
	return fn(ctx, message)
}

type retryPolicyMock struct {
	nextDelayFn func(context.Context, int, error) (time.Duration, bool)
}

func (m *retryPolicyMock) NextDelay(ctx context.Context, attempt int, err error) (time.Duration, bool) {
	return m.nextDelayFn(ctx, attempt, err)
}

func newTestWorker(
	repository Repository,
	registry SenderRegistry,
	retryPolicy RetryPolicy,
) *Worker {
	worker, err := New(repository, registry, retryPolicy, Config{
		PollInterval:    time.Hour,
		BatchSize:       10,
		Concurrency:     2,
		SendTimeout:     time.Second,
		LeaseTimeout:    5 * time.Minute,
		RequeueInterval: time.Hour,
	})
	if err != nil {
		panic(err)
	}
	return worker
}

func TestProcessTaskMarksSuccessfulDeliverySent(t *testing.T) {
	deliveryID := uuid.New()
	repository := &repositoryMock{
		markSentFn: func(_ context.Context, id uuid.UUID) error {
			assert.Equal(t, deliveryID, id)
			return nil
		},
	}
	registry := &senderRegistryMock{getFn: func(channelType notification.ChannelType) (sender.Sender, error) {
		assert.Equal(t, notification.ChannelTelegram, channelType)
		return senderFunc(func(context.Context, notification.Message) error {
			return nil
		}), nil
	}}
	retryPolicy := &retryPolicyMock{nextDelayFn: func(context.Context, int, error) (time.Duration, bool) {
		t.Fatal("retry policy must not be called after a successful send")
		return 0, false
	}}
	worker := newTestWorker(repository, registry, retryPolicy)

	err := worker.processTask(context.Background(), notification.DeliveryTask{
		Delivery: notification.Delivery{
			ID: deliveryID,
			Channel: notification.Channel{
				Type: notification.ChannelTelegram,
			},
		},
	})
	require.NoError(t, err)
}

func TestProcessTaskSchedulesRetry(t *testing.T) {
	deliveryID := uuid.New()
	now := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
	sendErr := errors.New("temporary failure")
	repository := &repositoryMock{
		retryFn: func(_ context.Context, id uuid.UUID, availableAt time.Time, reason string) error {
			assert.Equal(t, deliveryID, id)
			assert.Equal(t, now.Add(30*time.Second), availableAt)
			assert.Equal(t, sendErr.Error(), reason)
			return nil
		},
	}
	registry := &senderRegistryMock{getFn: func(notification.ChannelType) (sender.Sender, error) {
		return senderFunc(func(context.Context, notification.Message) error {
			return sendErr
		}), nil
	}}
	retryPolicy := &retryPolicyMock{nextDelayFn: func(_ context.Context, attempt int, err error) (time.Duration, bool) {
		assert.Equal(t, 2, attempt)
		assert.ErrorIs(t, err, sendErr)
		return 30 * time.Second, true
	}}
	worker := newTestWorker(repository, registry, retryPolicy)
	worker.now = func() time.Time { return now }

	err := worker.processTask(context.Background(), notification.DeliveryTask{
		Delivery: notification.Delivery{
			ID:       deliveryID,
			Attempts: 2,
		},
	})
	require.NoError(t, err)
}

func TestProcessTaskMarksPermanentFailure(t *testing.T) {
	deliveryID := uuid.New()
	sendErr := errors.New("invalid recipient")
	repository := &repositoryMock{
		failedFn: func(_ context.Context, id uuid.UUID, reason string) error {
			assert.Equal(t, deliveryID, id)
			assert.Equal(t, sendErr.Error(), reason)
			return nil
		},
	}
	registry := &senderRegistryMock{getFn: func(notification.ChannelType) (sender.Sender, error) {
		return senderFunc(func(context.Context, notification.Message) error {
			return sendErr
		}), nil
	}}
	retryPolicy := &retryPolicyMock{nextDelayFn: func(context.Context, int, error) (time.Duration, bool) {
		return 0, false
	}}
	worker := newTestWorker(repository, registry, retryPolicy)

	err := worker.processTask(context.Background(), notification.DeliveryTask{
		Delivery: notification.Delivery{ID: deliveryID},
	})
	require.NoError(t, err)
}

func TestProcessTasksRespectsConcurrencyLimit(t *testing.T) {
	const (
		taskCount   = 6
		concurrency = 2
	)

	started := make(chan struct{}, taskCount)
	release := make(chan struct{})
	var current atomic.Int32
	var maximum atomic.Int32

	registry := &senderRegistryMock{getFn: func(notification.ChannelType) (sender.Sender, error) {
		return senderFunc(func(context.Context, notification.Message) error {
			active := current.Add(1)
			for {
				oldMaximum := maximum.Load()
				if active <= oldMaximum || maximum.CompareAndSwap(oldMaximum, active) {
					break
				}
			}
			started <- struct{}{}
			<-release
			current.Add(-1)
			return nil
		}), nil
	}}
	repository := &repositoryMock{}
	retryPolicy := &retryPolicyMock{nextDelayFn: func(context.Context, int, error) (time.Duration, bool) {
		return 0, false
	}}
	worker := newTestWorker(repository, registry, retryPolicy)
	worker.config.Concurrency = concurrency

	tasks := make([]notification.DeliveryTask, taskCount)
	for index := range tasks {
		tasks[index].Delivery.ID = uuid.New()
	}

	done := make(chan error, 1)
	go func() {
		done <- worker.processTasks(context.Background(), tasks)
	}()

	for range concurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("notification send did not start")
		}
	}

	select {
	case <-started:
		t.Fatal("worker exceeded configured concurrency")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-done)
	assert.Equal(t, int32(concurrency), maximum.Load())
}

func TestRunStopsWithCancelledContext(t *testing.T) {
	worker := newTestWorker(
		&repositoryMock{},
		&senderRegistryMock{getFn: func(notification.ChannelType) (sender.Sender, error) {
			return nil, errors.New("not used")
		}},
		&retryPolicyMock{nextDelayFn: func(context.Context, int, error) (time.Duration, bool) {
			return 0, false
		}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, worker.Run(ctx))
}

func TestNewValidatesConfig(t *testing.T) {
	valid := DefaultConfig()
	repository := &repositoryMock{}
	registry := &senderRegistryMock{getFn: func(notification.ChannelType) (sender.Sender, error) {
		return nil, nil
	}}
	retryPolicy := &retryPolicyMock{nextDelayFn: func(context.Context, int, error) (time.Duration, bool) {
		return 0, false
	}}

	tests := []Config{
		{},
		func() Config { config := valid; config.BatchSize = 0; return config }(),
		func() Config { config := valid; config.Concurrency = 0; return config }(),
		func() Config { config := valid; config.SendTimeout = 0; return config }(),
		func() Config { config := valid; config.LeaseTimeout = 0; return config }(),
		func() Config { config := valid; config.RequeueInterval = 0; return config }(),
	}

	for _, config := range tests {
		_, err := New(repository, registry, retryPolicy, config)
		require.Error(t, err)
	}
}
