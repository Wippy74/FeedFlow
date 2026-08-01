package storage

import (
	"context"

	"github.com/google/uuid"
)

func (repo *Repository) FollowFeed(ctx context.Context, userID, feedID uuid.UUID) error {
	query := `INSERT INTO feed_follows (user_id, feed_id) VALUES ($1, $2)`

	_, err := repo.db.Exec(ctx, query, userID, feedID)
	if err != nil {
		return err
	}
	return nil
}
