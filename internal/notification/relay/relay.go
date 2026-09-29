package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Source string

const (
	SourceRequests Source = "requests"
	SourceDispatch Source = "dispatch"
)

type Record struct {
	ID      uuid.UUID
	Topic   string
	Key     string
	Value   []byte
	Headers map[string]string
}

type Publisher interface {
	Publish(context.Context, Record) error
}

type Relay struct {
	pool      *pgxpool.Pool
	publisher Publisher
	source    Source
	interval  time.Duration
}

func New(pool *pgxpool.Pool, publisher Publisher, source Source) (*Relay, error) {
	if pool == nil || publisher == nil {
		return nil, errors.New("relay requires a database and publisher")
	}
	if source != SourceRequests && source != SourceDispatch {
		return nil, fmt.Errorf("unknown relay source %q", source)
	}
	return &Relay{pool: pool, publisher: publisher, source: source, interval: time.Second}, nil
}

func (r *Relay) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		published, err := r.PublishOne(ctx)
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "outbox relay failed", "source", r.source, "error", err)
		}
		if published && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.interval):
		}
	}
	return nil
}

func (r *Relay) PublishOne(ctx context.Context) (published bool, resultErr error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin relay transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var record Record
	var payload []byte
	var eventType string
	var version int
	var kind string
	var attempts int
	if r.source == SourceRequests {
		err = tx.QueryRow(ctx, `SELECT id, target_topic, partition_key, payload, event_type, schema_version, attempts
			FROM outbox_events WHERE target_topic IS NOT NULL AND published_at IS NULL
			AND available_at <= NOW() ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(
			&record.ID, &record.Topic, &record.Key, &payload, &eventType, &version, &attempts)
	} else {
		err = tx.QueryRow(ctx, `SELECT id, target_topic, partition_key, payload, kind, publish_attempts
			FROM notification_dispatch_outbox WHERE published_at IS NULL AND available_at <= NOW()
			ORDER BY available_at, created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(
			&record.ID, &record.Topic, &record.Key, &payload, &kind, &attempts)
		eventType, version = "notification."+kind, 1
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select outbox record: %w", err)
	}
	if !json.Valid(payload) {
		return false, fmt.Errorf("outbox payload %s is invalid JSON", record.ID)
	}
	record.Value = payload
	record.Headers = map[string]string{
		"event_id": record.ID.String(), "event_type": eventType,
		"schema_version": fmt.Sprint(version), "content-type": "application/json",
	}
	publishCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	publishErr := r.publisher.Publish(publishCtx, record)
	cancel()
	if publishErr != nil {
		backoff := time.Duration(1<<min(attempts, 6)) * time.Second
		if r.source == SourceRequests {
			_, err = tx.Exec(ctx, `UPDATE outbox_events SET attempts = attempts + 1,
				available_at = NOW() + $2::bigint * interval '1 millisecond' WHERE id = $1`,
				record.ID, backoff.Milliseconds())
		} else {
			_, err = tx.Exec(ctx, `UPDATE notification_dispatch_outbox SET publish_attempts = publish_attempts + 1,
				available_at = NOW() + $2::bigint * interval '1 millisecond' WHERE id = $1`,
				record.ID, backoff.Milliseconds())
		}
		if err != nil {
			return false, fmt.Errorf("schedule outbox retry: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit outbox retry: %w", err)
		}
		return false, fmt.Errorf("publish outbox record %s: %w", record.ID, publishErr)
	}
	if r.source == SourceRequests {
		_, err = tx.Exec(ctx, `UPDATE outbox_events SET published_at = NOW(), processed_at = NOW(),
			attempts = attempts + 1 WHERE id = $1`, record.ID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE notification_dispatch_outbox SET published_at = NOW(),
			publish_attempts = publish_attempts + 1 WHERE id = $1`, record.ID)
	}
	if err != nil {
		return false, fmt.Errorf("mark outbox record published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit outbox publication: %w", err)
	}
	return true, nil
}
