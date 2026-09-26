package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/model"

	"github.com/google/uuid"
)

type Storage interface {
	SaveUser(ctx context.Context, id uuid.UUID, name, apiKey string) (model.User, error)
	AddFeed(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error)
	GetAllFeeds(ctx context.Context) ([]model.Feed, error)
	FollowFeed(ctx context.Context, userID, feedID uuid.UUID) error
	GetPosts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Post, error)
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
	tokenIssuer      TokenIssuer
	tokenVerifier    auth.TokenVerifier
	idGenerator      func() uuid.UUID
	apiKeyGenerator  func() (string, error)
	backgroundCtx    context.Context
	cancelBackground context.CancelFunc
	backgroundMu     sync.Mutex
	backgroundClosed bool
	backgroundWG     sync.WaitGroup
}

type TokenIssuer interface {
	Issue(uuid.UUID) (auth.AccessToken, error)
}

type Option func(*Handler)

func WithTokens(issuer TokenIssuer, verifier auth.TokenVerifier) Option {
	return func(h *Handler) {
		h.tokenIssuer = issuer
		h.tokenVerifier = verifier
	}
}

func NewHandler(storage Storage, cache Cache, options ...Option) *Handler {
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	h := &Handler{
		storage:          storage,
		cache:            cache,
		idGenerator:      uuid.New,
		apiKeyGenerator:  generateAPIKey,
		backgroundCtx:    backgroundCtx,
		cancelBackground: cancelBackground,
	}
	for _, option := range options {
		option(h)
	}
	return h
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
	router.HandleFunc("POST /v1/auth/token", h.PostToken)
	protected := auth.Middleware(h.tokenVerifier)
	router.Handle("POST /v1/feeds", protected(http.HandlerFunc(h.PostFeed)))
	router.HandleFunc("GET /v1/feeds", h.GetAllFeeds)
	router.Handle("POST /v1/feed_follows", protected(http.HandlerFunc(h.PostFollowFeed)))
	router.Handle("GET /v1/posts", protected(http.HandlerFunc(h.GetPosts)))
	return router
}
