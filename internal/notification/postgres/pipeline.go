package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"FeedFlow/internal/notification/contract"
	notification "FeedFlow/internal/notification/model"
	"FeedFlow/internal/notification/pipeline"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrStaleClaim = errors.New("notification delivery claim expired")

func retryTopic(delay time.Duration) string {
	switch {
	case delay <= time.Minute:
		return contract.Retry1mTopicV1
	case delay <= 10*time.Minute:
		return contract.Retry10mTopicV1
	default:
		return contract.Retry1hTopicV1
	}
}

func (repo *Repository) AcceptRequest(ctx context.Context, event contract.Requested, payload []byte) (resultErr error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	tag, err := tx.Exec(ctx, `INSERT INTO notification_inbox
		(event_id, notification_id, user_id, event_type, schema_version, template, payload, occurred_at)
		VALUES ($1, $1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (event_id) DO NOTHING`, event.EventID, event.UserID, event.EventType,
		event.SchemaVersion, event.Template, payload, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert notification inbox: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var same bool
		if err := tx.QueryRow(ctx, `SELECT user_id = $2 AND payload = $3::jsonb
			FROM notification_inbox WHERE event_id = $1`, event.EventID, event.UserID, payload).Scan(&same); err != nil {
			return fmt.Errorf("compare duplicate notification event: %w", err)
		}
		if !same {
			return pipeline.ErrIdentityConflict
		}
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO notification_deliveries (event_id, user_id, channel_id)
			SELECT $1, $2, id FROM (
				SELECT id FROM notification_channels WHERE user_id = $2 AND enabled = TRUE FOR SHARE
			) AS channels
			ON CONFLICT (event_id, channel_id) DO NOTHING`, event.EventID, event.UserID)
		if err != nil {
			return fmt.Errorf("create notification deliveries: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (repo *Repository) ResumeRetry(ctx context.Context, event contract.Retry) error {
	_, err := repo.pool.Exec(ctx, `UPDATE notification_deliveries AS d
		SET status = 'pending', available_at = NOW(), updated_at = NOW()
		FROM (SELECT id, user_id FROM notification_channels WHERE enabled = TRUE FOR SHARE) AS c
		WHERE d.id = $1 AND d.event_id = $2 AND d.user_id = $3 AND d.attempts = $4
		AND d.status = 'retry_wait' AND c.id = d.channel_id AND c.user_id = d.user_id`,
		event.DeliveryID, event.EventID, event.UserID, event.AttemptNo)
	return err
}

func (repo *Repository) Claim(ctx context.Context, limit int, lease time.Duration, channelTypes []string) ([]pipeline.Task, error) {
	if limit <= 0 || lease <= 0 {
		return nil, fmt.Errorf("invalid claim limit or lease")
	}
	rows, err := repo.pool.Query(ctx, `WITH selected AS (
		SELECT d.id FROM notification_deliveries AS d
		JOIN notification_channels AS c ON c.id = d.channel_id AND c.user_id = d.user_id AND c.enabled = TRUE
		WHERE d.status = 'pending' AND d.available_at <= NOW() AND c.channel_type = ANY($3::text[])
		ORDER BY d.available_at, d.created_at, d.id LIMIT $1 FOR UPDATE OF d, c SKIP LOCKED
	), claimed AS (
		UPDATE notification_deliveries AS d SET status = 'processing', attempts = attempts + 1,
		claim_token = gen_random_uuid(), lease_expires_at = NOW() + $2::bigint * interval '1 millisecond',
		updated_at = NOW() FROM selected AS s WHERE d.id = s.id
		RETURNING d.id, d.event_id, d.user_id, d.channel_id, d.claim_token, d.attempts
	)
	SELECT d.id, d.event_id, d.user_id, d.claim_token, d.attempts, c.channel_type,
		c.destination, i.payload
	FROM claimed AS d JOIN notification_channels AS c ON c.id = d.channel_id
	JOIN notification_inbox AS i ON i.event_id = d.event_id`, limit, lease.Milliseconds(), channelTypes)
	if err != nil {
		return nil, fmt.Errorf("claim deliveries: %w", err)
	}
	defer rows.Close()
	var tasks []pipeline.Task
	for rows.Next() {
		var task pipeline.Task
		var payload []byte
		var channelType string
		if err := rows.Scan(&task.ID, &task.EventID, &task.UserID, &task.ClaimToken,
			&task.Attempts, &channelType, &task.Message.Recipient, &payload); err != nil {
			return nil, err
		}
		var event contract.Requested
		if err := json.Unmarshal(payload, &event); err != nil {
			return nil, fmt.Errorf("decode inbox event %s: %w", task.EventID, err)
		}
		task.ChannelType = notification.ChannelType(channelType)
		task.Message.Title = event.Data.Title
		task.Message.Body = event.Data.Body
		task.Message.URL = event.Data.URL
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (repo *Repository) MarkSent(ctx context.Context, task pipeline.Task) error {
	tag, err := repo.pool.Exec(ctx, `UPDATE notification_deliveries SET status = 'sent', sent_at = NOW(),
		claim_token = NULL, lease_expires_at = NULL, last_error_code = NULL, updated_at = NOW()
		WHERE id = $1 AND claim_token = $2 AND status = 'processing'`, task.ID, task.ClaimToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleClaim
	}
	return nil
}

func (repo *Repository) MarkFailed(ctx context.Context, task pipeline.Task, delay time.Duration, errorCode string) error {
	if len(errorCode) > 100 {
		errorCode = "provider_error"
	}
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	active, err := channelActive(ctx, tx, task.ID)
	if err != nil {
		return err
	}
	if !active {
		tag, err := tx.Exec(ctx, `UPDATE notification_deliveries SET status = 'cancelled',
			claim_token = NULL, lease_expires_at = NULL, last_error_code = 'channel_disabled', updated_at = NOW()
			WHERE id = $1 AND claim_token = $2 AND status = 'processing'`, task.ID, task.ClaimToken)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrStaleClaim
		}
		return tx.Commit(ctx)
	}
	status, kind, topic := "failed", "dlq", contract.DLQTopicV1
	availableAt := time.Now().UTC()
	if delay > 0 {
		status, kind, topic = "retry_wait", "retry", retryTopic(delay)
		availableAt = availableAt.Add(delay)
	}
	tag, err := tx.Exec(ctx, `UPDATE notification_deliveries SET status = $3,
		claim_token = NULL, lease_expires_at = NULL, last_error_code = $4, updated_at = NOW()
		WHERE id = $1 AND claim_token = $2 AND status = 'processing'`,
		task.ID, task.ClaimToken, status, errorCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleClaim
	}
	payload := dispatchPayload(task, errorCode)
	_, err = tx.Exec(ctx, `INSERT INTO notification_dispatch_outbox
		(delivery_id, user_id, attempt_no, kind, target_topic, payload, available_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (delivery_id, kind, attempt_no) DO NOTHING`,
		task.ID, task.UserID, task.Attempts, kind, topic, payload, availableAt)
	if err != nil {
		return fmt.Errorf("insert dispatch outbox: %w", err)
	}
	return tx.Commit(ctx)
}

func channelActive(ctx context.Context, tx pgx.Tx, deliveryID uuid.UUID) (bool, error) {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT c.enabled FROM notification_deliveries AS d
		JOIN notification_channels AS c ON c.id = d.channel_id AND c.user_id = d.user_id
		WHERE d.id = $1 FOR SHARE OF c`, deliveryID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func dispatchPayload(task pipeline.Task, errorCode string) []byte {
	payload, _ := json.Marshal(struct {
		DeliveryID uuid.UUID `json:"delivery_id"`
		EventID    uuid.UUID `json:"event_id"`
		UserID     uuid.UUID `json:"user_id"`
		AttemptNo  int       `json:"attempt_no"`
		ErrorCode  string    `json:"error_code,omitempty"`
	}{task.ID, task.EventID, task.UserID, task.Attempts, errorCode})
	return payload
}

func (repo *Repository) RecoverExpired(ctx context.Context, limit, maxAttempts int) (int, error) {
	if limit <= 0 || maxAttempts <= 0 {
		return 0, fmt.Errorf("invalid recovery limit or max attempts")
	}
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	rows, err := tx.Query(ctx, `SELECT id, event_id, user_id, attempts FROM notification_deliveries
		WHERE status = 'processing' AND lease_expires_at < NOW()
		ORDER BY lease_expires_at, id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	var tasks []pipeline.Task
	for rows.Next() {
		var task pipeline.Task
		if err := rows.Scan(&task.ID, &task.EventID, &task.UserID, &task.Attempts); err != nil {
			rows.Close()
			return 0, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, task := range tasks {
		active, err := channelActive(ctx, tx, task.ID)
		if err != nil {
			return 0, err
		}
		status := "pending"
		if !active {
			status = "cancelled"
		} else if task.Attempts >= maxAttempts {
			status = "failed"
		}
		errorCode := "lease_expired"
		if !active {
			errorCode = "channel_disabled"
		}
		if _, err := tx.Exec(ctx, `UPDATE notification_deliveries SET status = $2,
			claim_token = NULL, lease_expires_at = NULL, available_at = NOW(),
			last_error_code = $3, updated_at = NOW() WHERE id = $1`, task.ID, status, errorCode); err != nil {
			return 0, err
		}
		if status == "failed" {
			_, err := tx.Exec(ctx, `INSERT INTO notification_dispatch_outbox
				(delivery_id, user_id, attempt_no, kind, target_topic, payload)
				VALUES ($1, $2, $3, 'dlq', $4, $5::jsonb)
				ON CONFLICT (delivery_id, kind, attempt_no) DO NOTHING`,
				task.ID, task.UserID, task.Attempts, contract.DLQTopicV1, dispatchPayload(task, "lease_expired"))
			if err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(tasks), nil
}
