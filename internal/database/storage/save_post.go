package storage

import (
	"context"
	"errors"
	"time"

	"NewsAggregator/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *Repository) SavePost(ctx context.Context, post model.Post) (bool, error) {
	query := `INSERT INTO posts (id, created_at, updated_at, title, description, published_at, url, feed_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (url) DO NOTHING RETURNING id;`

	now := time.Now().UTC()
	var insertedID uuid.UUID
	err := repo.db.QueryRow(ctx, query, post.ID, now, now, post.Title, post.Description, post.PublishedAt, post.Url, post.FeedID).Scan(&insertedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
