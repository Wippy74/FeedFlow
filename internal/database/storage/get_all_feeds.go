package storage

import (
	"context"

	"FeedFlow/internal/model"
)

func (repo *Repository) GetAllFeeds(ctx context.Context) ([]model.Feed, error) {
	query := `SELECT * FROM feeds`

	var feeds []model.Feed
	res, err := repo.db.Query(ctx, query)
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
