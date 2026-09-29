package storage

import (
	"context"
	"fmt"

	"FeedFlow/internal/model"
	"FeedFlow/internal/notification/contract"
)

const notificationProducer = "feedflow-monolith"

func (repo *Repository) SavePost(ctx context.Context, post model.Post) (bool, error) {
	now := repo.now().UTC()
	query := `WITH inserted_post AS (
	INSERT INTO posts (
		id, created_at, updated_at, title, description, published_at, url, feed_id
	)
	VALUES ($1, $2, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (url) DO NOTHING
	RETURNING id, feed_id
	),
	intent_candidates AS MATERIALIZED (
		SELECT gen_random_uuid() AS id, ff.user_id
		FROM inserted_post AS p
		JOIN feed_follows AS ff ON ff.feed_id = p.feed_id
	),
	inserted_intents AS (
		INSERT INTO outbox_events (
			id,
			event_type,
			aggregate_id,
			payload,
			created_at,
			available_at,
			target_topic,
			partition_key,
			schema_version
		)
		SELECT
			c.id,
			$8::text,
			$1,
			jsonb_build_object(
				'event_id', c.id,
				'event_type', $8::text,
				'schema_version', $10::integer,
				'occurred_at', $2::timestamptz,
				'producer', $12::text,
				'notification_id', c.id,
				'user_id', c.user_id,
				'template', $11::text,
				'data', jsonb_build_object(
					'post_id', $1,
					'feed_id', $7,
					'title', $3,
					'body', COALESCE($4::text, ''),
					'url', $6,
					'published_at', $5::timestamptz
				)
			),
			$2,
			$2,
			$9,
			c.user_id::text,
			$10
		FROM intent_candidates AS c
		RETURNING id
	)
	SELECT EXISTS (SELECT 1 FROM inserted_post)`

	var inserted bool
	err := repo.db.QueryRow(
		ctx,
		query,
		post.ID,
		now,
		post.Title,
		post.Description,
		post.PublishedAt,
		post.Url,
		post.FeedID,
		contract.NotificationRequested,
		contract.RequestsTopicV1,
		contract.RequestedSchemaV1,
		contract.TemplateNewPost,
		notificationProducer,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("save post with outbox event: %w", err)
	}

	return inserted, nil
}
