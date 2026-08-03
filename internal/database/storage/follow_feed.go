package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

func (repo *Repository) FollowFeed(ctx context.Context, userID, feedID uuid.UUID) error {
	query := `
		INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id) 
		VALUES ($1, $2, $3, $4, $5)
	`

	newID := uuid.New()
	now := time.Now().UTC()
	_, err := repo.db.Exec(ctx, query, newID, now, now, userID, feedID)
	if err != nil {
		return err
	}
	return nil
}
