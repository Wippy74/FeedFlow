package storage

import (
	"NewsAggregator/internal/model"
	"context"
	"time"
)

func (repo *Repository) SavePost(ctx context.Context, post model.Post) error {
	query := `INSERT INTO posts (id, created_at, updated_at, title, description, published_at, url, feed_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (url) DO NOTHING;`

	now := time.Now().UTC()
	_, err := repo.db.Exec(ctx, query, post.ID, now, now, post.Title, post.Description, post.PublishedAt, post.Url, post.FeedID)
	return err
}
