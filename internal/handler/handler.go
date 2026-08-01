package handler

import (
	"NewsAggregator/internal/model"
	"context"
	"net/http"

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

type Handler struct {
	storage Storage
}

func NewHandler(storage Storage) *Handler {
	return &Handler{
		storage: storage,
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
