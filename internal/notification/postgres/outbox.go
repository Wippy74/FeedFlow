package postgres

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (repo *Repository) ExpandPostCreatedEvents(ctx context.Context, limit int) (processed int64, resultErr error) {
	query := `WITH claimed AS MATERIALIZED (
		SELECT oe.id, oe.aggregate_id FROM outbox_events AS oe
		WHERE oe.processed_at IS NULL AND oe.available <= NOW() AND oe.event_type = $1
		ORDER BY oe.created_at, oe.id
		LIMIT $2
		FOR UPDATE OF oe SKIP LOCKED
	),
	created_deliveries AS (
		INSERT INTO notification_deliveries (
			user_id,
			post_id,
			channel_id,
			status,
			attempts,
			available_at,
			created_at,
			updated_at
		)
		SELECT
			ff.user_id,
			p.id,
			nc.id,
			'pending',
			0,
			NOW(),
			NOW(),
			NOW()
		FROM claimed AS c
		JOIN posts AS p ON p.id = c.aggregate_id
		JOIN feed_follows AS ff ON ff.feed_id = p.feed_id
		JOIN notification_channels AS nc ON nc.user_id = ff.user_id AND nc.enabled = TRUE
		ON CONFLICT (user_id, post_id, channel_id) DO NOTHING
		RETURNING id
	)
	UPDATE outbox_events AS oe
	SET
		processed_at = NOW(),
		attempts = oe.attempts + 1
	FROM claimed AS c
	WHERE oe.id = c.id
	`

	if limit <= 0 {
		return 0, fmt.Errorf("outbox batch limit must be greater than zero")
	}

	tx, err := repo.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin outbox transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	tag, err := tx.Exec(ctx, query, notification.EventPostCreated, limit)
	if err != nil {
		return 0, fmt.Errorf("expand post-created events: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outbox transaction: %w", err)
	}
	return tag.RowsAffected(), nil
}
