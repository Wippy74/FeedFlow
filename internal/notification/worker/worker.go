package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	notification "FeedFlow/internal/notification/model"
	sender "FeedFlow/internal/notification/sender"

	"github.com/google/uuid"
)

const (
	defaultPollInterval    = time.Second
	defaultBatchSize       = 50
	defaultConcurrency     = 5
	defaultSendTimeout     = 15 * time.Second
	defaultLeaseTimeout    = 5 * time.Minute
	defaultRequeueInterval = time.Minute
)

type Repository interface {
	ExpandPostCreatedEvents(ctx context.Context, limit int) (int64, error)
	ClaimDeliveries(ctx context.Context, limit int) ([]notification.DeliveryTask, error)
	MarkDeliverySent(ctx context.Context, id uuid.UUID) error
	ScheduleDeliveryRetry(ctx context.Context, id uuid.UUID, availableAt time.Time, reason string) error
	MarkDeliveryFailed(ctx context.Context, id uuid.UUID, reason string) error
	RequeueStaleDeliveries(ctx context.Context, staleBefore time.Time) (int64, error)
}

type SenderRegistry interface {
	Get(channelType notification.ChannelType) (sender.Sender, error)
}

type RetryPolicy interface {
	NextDelay(ctx context.Context, attempt int, err error) (time.Duration, bool)
}

type Config struct {
	PollInterval    time.Duration
	BatchSize       int
	Concurrency     int
	SendTimeout     time.Duration
	LeaseTimeout    time.Duration
	RequeueInterval time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval:    defaultPollInterval,
		BatchSize:       defaultBatchSize,
		Concurrency:     defaultConcurrency,
		SendTimeout:     defaultSendTimeout,
		LeaseTimeout:    defaultLeaseTimeout,
		RequeueInterval: defaultRequeueInterval,
	}
}

type Worker struct {
	repository Repository
	senders    SenderRegistry
	retry      RetryPolicy
	config     Config
	now        func() time.Time
}

func New(repository Repository, senders SenderRegistry, retryPolicy RetryPolicy, config Config) (*Worker, error) {
	if repository == nil {
		return nil, fmt.Errorf("notification repository must not be nil")
	}
	if senders == nil {
		return nil, fmt.Errorf("notification sender registry must not be nil")
	}
	if retryPolicy == nil {
		return nil, fmt.Errorf("notification retry policy must not be nil")
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return &Worker{
		repository: repository,
		senders:    senders,
		retry:      retryPolicy,
		config:     config,
		now:        time.Now,
	}, nil
}

func validateConfig(config Config) error {
	if config.PollInterval <= 0 {
		return fmt.Errorf("notification poll interval must be greater than zero")
	}
	if config.BatchSize <= 0 {
		return fmt.Errorf("notification batch size must be greater than zero")
	}
	if config.Concurrency <= 0 {
		return fmt.Errorf("notification concurrency must be greater than zero")
	}
	if config.SendTimeout <= 0 {
		return fmt.Errorf("notification send timeout must be greater than zero")
	}
	if config.LeaseTimeout <= 0 {
		return fmt.Errorf("notification lease timeout must be greater than zero")
	}
	if config.RequeueInterval <= 0 {
		return fmt.Errorf("notification requeue interval must be greater than zero")
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("notification worker context must not be nil")
	}
	if ctx.Err() != nil {
		return nil
	}

	if err := w.requeueStale(ctx); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "failed to requeue stale notification deliveries", "error", err)
	}

	pollTicker := time.NewTicker(w.config.PollInterval)
	defer pollTicker.Stop()
	requeueTicker := time.NewTicker(w.config.RequeueInterval)
	defer requeueTicker.Stop()

	for {
		if err := w.processBatch(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "failed to process notification batch", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-pollTicker.C:
		case <-requeueTicker.C:
			if err := w.requeueStale(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "failed to requeue stale notification deliveries", "error", err)
			}
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) error {
	var batchErr error

	if _, err := w.repository.ExpandPostCreatedEvents(ctx, w.config.BatchSize); err != nil {
		batchErr = errors.Join(batchErr, fmt.Errorf("expand notification outbox events: %w", err))
	}
	if ctx.Err() != nil {
		return errors.Join(batchErr, ctx.Err())
	}

	tasks, err := w.repository.ClaimDeliveries(ctx, w.config.BatchSize)
	if err != nil {
		return errors.Join(batchErr, fmt.Errorf("claim notification deliveries: %w", err))
	}

	return errors.Join(batchErr, w.processTasks(ctx, tasks))
}

func (w *Worker) processTasks(ctx context.Context, tasks []notification.DeliveryTask) error {
	if len(tasks) == 0 {
		return nil
	}

	workerCount := min(w.config.Concurrency, len(tasks))
	jobs := make(chan notification.DeliveryTask)
	resultErrors := make(chan error, len(tasks))

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for task := range jobs {
				if err := w.processTask(ctx, task); err != nil {
					resultErrors <- fmt.Errorf("process notification delivery %s: %w", task.Delivery.ID, err)
				}
			}
		}()
	}

enqueueLoop:
	for _, task := range tasks {
		select {
		case jobs <- task:
		case <-ctx.Done():
			break enqueueLoop
		}
	}
	close(jobs)
	wg.Wait()
	close(resultErrors)

	var resultErr error
	for err := range resultErrors {
		resultErr = errors.Join(resultErr, err)
	}
	if ctx.Err() != nil {
		resultErr = errors.Join(resultErr, ctx.Err())
	}
	return resultErr
}

func (w *Worker) processTask(ctx context.Context, task notification.DeliveryTask) error {
	channelSender, err := w.senders.Get(task.Delivery.Channel.Type)
	if err != nil {
		return w.repository.MarkDeliveryFailed(ctx, task.Delivery.ID, err.Error())
	}

	sendCtx, cancelSend := context.WithTimeout(ctx, w.config.SendTimeout)
	sendErr := channelSender.Send(sendCtx, task.Message)
	cancelSend()

	if sendErr == nil {
		return w.repository.MarkDeliverySent(ctx, task.Delivery.ID)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	delay, shouldRetry := w.retry.NextDelay(ctx, task.Delivery.Attempts, sendErr)
	if shouldRetry {
		return w.repository.ScheduleDeliveryRetry(ctx, task.Delivery.ID, w.now().UTC().Add(delay), sendErr.Error())
	}

	return w.repository.MarkDeliveryFailed(ctx, task.Delivery.ID, sendErr.Error())
}

func (w *Worker) requeueStale(ctx context.Context) error {
	staleBefore := w.now().UTC().Add(-w.config.LeaseTimeout)
	requeued, err := w.repository.RequeueStaleDeliveries(ctx, staleBefore)
	if err != nil {
		return err
	}
	if requeued > 0 {
		slog.InfoContext(ctx, "stale notification deliveries requeued", "count", requeued)
	}
	return nil
}
