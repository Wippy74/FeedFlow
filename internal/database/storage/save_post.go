package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"FeedFlow/internal/model"
	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
)

const (
	postCreatedSchemaVersion = 1
	postCreatedProducer      = "feedflow-monolith"
)

type postCreatedPayload struct {
	EventID       uuid.UUID              `json:"event_id"`
	EventType     notification.EventType `json:"event_type"`
	SchemaVersion int                    `json:"schema_version"`
	OccurredAt    time.Time              `json:"occurred_at"`
	Producer      string                 `json:"producer"`
	Data          postCreatedData        `json:"data"`
}

type postCreatedData struct {
	PostID      uuid.UUID `json:"post_id"`
	FeedID      uuid.UUID `json:"feed_id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	PublishedAt time.Time `json:"published_at"`
	URL         string    `json:"url"`
}

func (repo *Repository) SavePost(ctx context.Context, post model.Post) (bool, error) {
	now := repo.now().UTC()
	eventID := repo.idGenerator()

	var description *string
	if post.Description.Valid {
		description = &post.Description.String
	}

	payload, err := json.Marshal(postCreatedPayload{
		EventID:       eventID,
		EventType:     notification.EventPostCreated,
		SchemaVersion: postCreatedSchemaVersion,
		OccurredAt:    now,
		Producer:      postCreatedProducer,
		Data: postCreatedData{
			PostID:      post.ID,
			FeedID:      post.FeedID,
			Title:       post.Title,
			Description: description,
			PublishedAt: post.PublishedAt.UTC(),
			URL:         post.Url,
		},
	})
	if err != nil {
		return false, fmt.Errorf("marshal post-created outbox payload: %w", err)
	}

	query := `WITH inserted_post AS (
		INSERT INTO posts (
		id,
		created_at,
		updated_at,
		title,
		description,
		published_at,
		url,
		feed_id
	)
	VALUES ($1, $2, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (url) DO NOTHING
	RETURNING id
	),
	inserted_event AS (
		INSERT INTO outbox_events (
		id,
		event_type,
		aggregate_id,
		payload,
		created_at,
		available_at
	)
	SELECT $8, $9, id, $10, $2, $2
	FROM inserted_post
	RETURNING id
	)
	SELECT EXISTS (SELECT 1 FROM inserted_post)`

	var inserted bool
	err = repo.db.QueryRow(
		ctx,
		query,
		post.ID,
		now,
		post.Title,
		post.Description,
		post.PublishedAt,
		post.Url,
		post.FeedID,
		eventID,
		notification.EventPostCreated,
		payload,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("save post with outbox event: %w", err)
	}

	return inserted, nil
}
