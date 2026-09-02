package handler

import (
	"NewsAggregator/internal/model"
	"context"
	"net/http"
	"sync"
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
	SetFeeds(ctx context.Context, key string, feeds []model.Feed, ttl time.Duration) error
	GetFeeds(ctx context.Context, key string) ([]model.Feed, error)
	Delete(ctx context.Context, key string) error
}

type Handler struct {
	storage          Storage
	cache            Cache
	idGenerator      func() uuid.UUID
	apiKeyGenerator  func() (string, error)
	backgroundCtx    context.Context
	cancelBackground context.CancelFunc
	backgroundMu     sync.Mutex
	backgroundClosed bool
	backgroundWG     sync.WaitGroup
}

func NewHandler(storage Storage, cache Cache) *Handler {
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	return &Handler{
		storage:          storage,
		cache:            cache,
		idGenerator:      uuid.New,
		apiKeyGenerator:  generateAPIKey,
		backgroundCtx:    backgroundCtx,
		cancelBackground: cancelBackground,
	}
}

func (h *Handler) runInBackground(task func(context.Context)) {
	h.backgroundMu.Lock()
	if h.backgroundClosed {
		h.backgroundMu.Unlock()
		return
	}
	h.backgroundWG.Add(1)
	h.backgroundMu.Unlock()

	go func() {
		defer h.backgroundWG.Done()
		task(h.backgroundCtx)
	}()
}

func (h *Handler) Shutdown(ctx context.Context) error {
	h.backgroundMu.Lock()
	h.backgroundClosed = true
	h.cancelBackground()
	h.backgroundMu.Unlock()

	done := make(chan struct{})
	go func() {
		h.backgroundWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
