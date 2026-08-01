package storage

import (
	"NewsAggregator/internal/model"
	"context"
	"time"

	"github.com/google/uuid"
)

func (repo *Repository) AddFeed(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error) {
	query := `INSERT INTO feeds (id, created_at, updated_at, name, url, last_fetched_at) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at, updated_at, name, url, last_fetched_at`

	var newFeed model.Feed

	err := repo.db.QueryRow(ctx, query, id, time.Now().UTC(), time.Now().UTC(), name, url, time.Now().UTC()).Scan(
		&newFeed.ID, &newFeed.CreatedAt, &newFeed.UpdatedAt, &newFeed.Name, &newFeed.Url, &newFeed.LastFetchedAt)
	if err != nil {
		return model.Feed{}, err
	}

	return newFeed, nil
}
