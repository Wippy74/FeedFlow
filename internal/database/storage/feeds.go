package storage

import (
	"NewsAggregator/internal/model"
	"context"
	"time"

	"github.com/google/uuid"
)

func (repo *Repository) GetNextFeedsToFetch(ctx context.Context, limit int) ([]model.Feed, error) {
	query := `SELECT id, name, url, last_fetched_at FROM feeds ORDER BY last_fetched_at ASC NULLS FIRST LIMIT $1`

	var feeds []model.Feed
	raws, err := repo.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer raws.Close()

	for raws.Next() {
		var feed model.Feed
		err := raws.Scan(&feed.ID, &feed.Name, &feed.Url, &feed.LastFetchedAt)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, feed)
	}
	return feeds, nil
}

func (repo *Repository) MarkFeedFetched(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE feeds SET last_fetched_at = $1 WHERE id = $2`

	_, err := repo.db.Exec(ctx, query, time.Now(), id)
	return err
}
