package storage

import (
	"NewsAggregator/internal/model"
	"context"

	"github.com/google/uuid"
)

func (repo *Repository) GetPosts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Post, error) {
	query := `SELECT posts.id, posts.title, posts.description, posts.published_at, posts.url, posts.feed_id FROM posts JOIN feed_follows ON posts.feed_id = feed_follows.feed_id
         WHERE feed_follows.user_id = $1 
         ORDER BY posts.published_at DESC LIMIT $2 OFFSET $3`

	posts := []model.Post{}
	raws, err := repo.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer raws.Close()

	for raws.Next() {
		var post model.Post
		err := raws.Scan(&post.ID, &post.Title, &post.Description, &post.PublishedAt, &post.Url, &post.FeedID)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	if err := raws.Err(); err != nil {
		return nil, err
	}
	return posts, nil

}
