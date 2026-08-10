package handler

import (
	"NewsAggregator/internal/model"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Storage interface {
	SaveUser(ctx context.Context, id uuid.UUID, name, apiKey string) (model.User, error)
	AddFeed(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error)
	GetAllFeeds(ctx context.Context) ([]model.Feed, error)
	FollowFeed(ctx context.Context, userID, feedID uuid.UUID) error
	GetPosts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Post, error)
	SavePost(ctx context.Context, post model.Post) error
	GetUserByApiKey(ctx context.Context, apiKey string) (model.User, error)
}

type Cache interface {
	SetPost(ctx context.Context, key string, posts []model.Post, ttl time.Duration) error
	GetPost(ctx context.Context, key string) ([]model.Post, error)
	SetUser(ctx context.Context, key string, user model.User, ttl time.Duration) error
	GetUser(ctx context.Context, key string) (model.User, error)
}

type Handler struct {
	storage Storage
	cache   Cache
}

func NewHandler(storage Storage, cache Cache) *Handler {
	return &Handler{
		storage: storage,
		cache:   cache,
	}
}

func (h *Handler) InitRouter() *http.ServeMux {
	router := http.NewServeMux()

	router.HandleFunc("POST /v1/users", h.PostUser)
	router.HandleFunc("POST /v1/feeds", h.AuthMiddleware(h.PostFeed))
	router.HandleFunc("GET /v1/feeds", h.GetAllFeeds)
	router.HandleFunc("POST /v1/feed_follows", h.AuthMiddleware(h.PostFollowFeed))
	router.HandleFunc("GET /v1/posts", h.AuthMiddleware(h.GetPosts))

	return router
}
