package storage

import (
	"context"
	"errors"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
)

func (repo *Repository) GetFeedsPage(ctx context.Context, after *uuid.UUID, limit int) ([]model.Feed, error) {
	if limit < 1 || limit > 101 {
		return nil, errors.New("feeds page limit must be between 1 and 101")
	}

	query := `SELECT id, created_at, updated_at, name, url, last_fetched_at
		FROM feeds ORDER BY id ASC LIMIT $1`
	args := []any{limit}
	if after != nil {
		query = `SELECT id, created_at, updated_at, name, url, last_fetched_at
			FROM feeds WHERE id > $1 ORDER BY id ASC LIMIT $2`
		args = []any{*after, limit}
	}

	var feeds []model.Feed
	res, err := repo.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	for res.Next() {
		var feed model.Feed
		if err := res.Scan(&feed.ID, &feed.CreatedAt, &feed.UpdatedAt, &feed.Name, &feed.Url, &feed.LastFetchedAt); err != nil {
			return nil, err
		}
		feeds = append(feeds, feed)
	}

	if err := res.Err(); err != nil {
		return nil, err
	}

	return feeds, nil
}
