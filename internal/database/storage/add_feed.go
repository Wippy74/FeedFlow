package storage

import (
	"context"
	"time"

	"NewsAggregator/internal/model"

	"github.com/google/uuid"
)

func (repo *Repository) AddFeed(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error) {
	query := `INSERT INTO feeds (id, created_at, updated_at, name, url, last_fetched_at) VALUES ($1, $2, $3, $4, $5, NULL) RETURNING id, created_at, updated_at, name, url, last_fetched_at`

	var newFeed model.Feed

	now := time.Now().UTC()
	err := repo.db.QueryRow(ctx, query, id, now, now, name, url).Scan(
		&newFeed.ID, &newFeed.CreatedAt, &newFeed.UpdatedAt, &newFeed.Name, &newFeed.Url, &newFeed.LastFetchedAt)
	if err != nil {
		return model.Feed{}, err
	}

	return newFeed, nil
}
