package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	_ "github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
	ApiKey    string    `json:"api_key"`
}

type Feed struct {
	ID            uuid.UUID    `json:"id"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	Name          string       `json:"name"`
	Url           string       `json:"url"`
	LastFetchedAt sql.NullTime `json:"last_fetched_at"`
}

type FeedFollows struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uuid.UUID `json:"user_id"`
	FeedID    uuid.UUID `json:"feed_id"`
}

type Post struct {
	ID          uuid.UUID      `json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Title       string         `json:"title"`
	Description sql.NullString `json:"description"`
	PublishedAt time.Time      `json:"published_at"`
	Url         string         `json:"url"`
	FeedID      uuid.UUID      `json:"feed_id"`
}
