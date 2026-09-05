package postgres

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (repo *Repository) ClaimDeliveries(ctx context.Context, limit int) ([]notification.DeliveryTask, error) {
	query := `WITH selected AS (
		SELECT d.id
		FROM notification_deliveries AS d
		JOIN notification_channels AS nc
			ON nc.id = d.channel_id
		   AND nc.enabled = TRUE
		WHERE d.status = 'pending'
		  AND d.available_at <= NOW()
		ORDER BY d.available_at, d.created_at, d.id
		LIMIT $1
		FOR UPDATE OF d SKIP LOCKED
		),
	claimed AS (
		UPDATE notification_deliveries AS d
		SET
			status = 'processing',
			attempts = d.attempts + 1,
			updated_at = NOW()
		FROM selected AS s
		WHERE d.id = s.id
		RETURNING
			d.id,
			d.user_id,
			d.post_id,
			d.channel_id,
			d.attempts
	)
	SELECT
		c.id,
		c.user_id,
		c.post_id,
		c.channel_id,
		c.attempts,
		nc.channel_type,
		nc.destination,
		p.title,
		COALESCE(p.description, ''),
		p.url
	FROM claimed AS c
	JOIN notification_channels AS nc
		ON nc.id = c.channel_id
	JOIN posts AS p
		ON p.id = c.post_id
	ORDER BY c.id
	`

	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}

	rows, err := repo.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim notification deliveries: %w", err)
	}
	defer rows.Close()

	tasks := make([]notification.DeliveryTask, 0, limit)
	for rows.Next() {
		var task notification.DeliveryTask
		var channelType string

		err := rows.Scan(
			&task.Delivery.ID,
			&task.Delivery.UserID,
			&task.Delivery.PostID,
			&task.Delivery.Channel.ID,
			&task.Delivery.Attempts,
			&channelType,
			&task.Delivery.Channel.Destination,
			&task.Message.Title,
			&task.Message.Body,
			&task.Message.URL,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification delivery: %w", err)
		}
		task.Delivery.Status = notification.StatusProcessing
		task.Delivery.Channel.UserID = task.Delivery.UserID
		task.Delivery.Channel.Type = notification.ChannelType(channelType)
		task.Delivery.Channel.Enabled = true
		task.Message.Recipient = task.Delivery.Channel.Destination
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan notification deliveries: %w", err)
	}
	return tasks, rows.Err()
}

func (repo *Repository) MarkDeliverySent(ctx context.Context, deliveryID uuid.UUID) error {
	query := `UPDATE notification_deliveries SET status = 'sent', sent_at = NOW(), 
            last_error = NULL, updated_at = NOW() WHERE id = $1 AND status = 'processing'`

	tag, err := repo.pool.Exec(ctx, query, deliveryID)
	if err != nil {
		return fmt.Errorf("mark notification delivery %s as sent: %w", deliveryID, err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery %s is not in processing state", deliveryID)
	}
	return nil
}

func (repo *Repository) MarkDeliveryFailed(ctx context.Context, deliveryID uuid.UUID, reason string) error {
	query := `UPDATE notification_deliveries SET status = 'failed', last_error = LEFT($2, 2000), updated_at = NOW() WHERE id = $1 AND status = 'processing'`

	tag, err := repo.pool.Exec(ctx, query, deliveryID, reason)
	if err != nil {
		return fmt.Errorf("mark notification delivery %s as failed: %w", deliveryID, err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery %s is not in processing state", deliveryID)
	}
	return nil
}

func (repo *Repository) ScheduleDeliveryRetry(ctx context.Context, id uuid.UUID, availableAt time.Time, reason string) error {
	query := `UPDATE notification_deliveries SET status = 'pending', available_at = $2, last_error = LEFT($3, 2000), updated_at = NOW() WHERE id = $1 AND status = 'processing'`

	tag, err := repo.pool.Exec(ctx, query, id, availableAt.UTC(), reason)
	if err != nil {
		return fmt.Errorf("schedule notification delivery %s retry: %w", id, err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("notification delivery %s is not in processing state", id)
	}
	return nil
}

func (repo *Repository) RequeueStaleDeliveries(ctx context.Context, staleBefore time.Time) (int64, error) {
	query := `UPDATE notification_deliveries SET status = 'pending', available_at = NOW(), 
    			last_error = 'processing lease expired', updated_at = NOW() WHERE status = 'processing' AND updated_at < $1`

	tag, err := repo.pool.Exec(ctx, query, staleBefore)
	if err != nil {
		return 0, fmt.Errorf("requeue notification deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}
