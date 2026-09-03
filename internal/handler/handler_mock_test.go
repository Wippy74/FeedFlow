package handler

import (
	"context"
	"time"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
)

type MockStorage struct {
	SaveUserFn        func(ctx context.Context, id uuid.UUID, name, apiKey string) (model.User, error)
	AddFeedFn         func(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error)
	GetAllFeedsFn     func(ctx context.Context) ([]model.Feed, error)
	FollowFeedFn      func(ctx context.Context, userID, feedID uuid.UUID) error
	GetPostsFn        func(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Post, error)
	SavePostFn        func(ctx context.Context, post model.Post) error
	GetUserByApiKeyFn func(ctx context.Context, apiKey string) (model.User, error)
}

func (m *MockStorage) SaveUser(ctx context.Context, id uuid.UUID, name, apiKey string) (model.User, error) {
	if m.SaveUserFn != nil {
		return m.SaveUserFn(ctx, id, name, apiKey)
	}
	return model.User{}, nil
}

func (m *MockStorage) AddFeed(ctx context.Context, id uuid.UUID, name, url string) (model.Feed, error) {
	if m.AddFeedFn != nil {
		return m.AddFeedFn(ctx, id, name, url)
	}
	return model.Feed{}, nil
}

func (m *MockStorage) GetAllFeeds(ctx context.Context) ([]model.Feed, error) {
	if m.GetAllFeedsFn != nil {
		return m.GetAllFeedsFn(ctx)
	}
	return []model.Feed{}, nil
}

func (m *MockStorage) FollowFeed(ctx context.Context, userID, feedID uuid.UUID) error {
	if m.FollowFeedFn != nil {
		return m.FollowFeedFn(ctx, userID, feedID)
	}
	return nil
}

func (m *MockStorage) GetPosts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Post, error) {
	if m.GetPostsFn != nil {
		return m.GetPostsFn(ctx, userID, limit, offset)
	}
	return []model.Post{}, nil
}

func (m *MockStorage) SavePost(ctx context.Context, post model.Post) error {
	if m.SavePostFn != nil {
		return m.SavePostFn(ctx, post)
	}
	return nil
}

func (m *MockStorage) GetUserByApiKey(ctx context.Context, apiKey string) (model.User, error) {
	if m.GetUserByApiKeyFn != nil {
		return m.GetUserByApiKeyFn(ctx, apiKey)
	}
	return model.User{}, nil
}

type MockCache struct {
	SetPostFn  func(ctx context.Context, key string, posts []model.Post, ttl time.Duration) error
	GetPostFn  func(ctx context.Context, key string) ([]model.Post, error)
	SetUserFn  func(ctx context.Context, key string, user model.User, ttl time.Duration) error
	GetUserFn  func(ctx context.Context, key string) (model.User, error)
	SetFeedsFn func(ctx context.Context, key string, feeds []model.Feed, ttl time.Duration) error
	GetFeedsFn func(ctx context.Context, key string) ([]model.Feed, error)
	DeleteFn   func(ctx context.Context, key string) error
}

func (m *MockCache) SetPost(ctx context.Context, key string, posts []model.Post, ttl time.Duration) error {
	if m.SetPostFn != nil {
		return m.SetPostFn(ctx, key, posts, ttl)
	}
	return nil
}

func (m *MockCache) GetPost(ctx context.Context, key string) ([]model.Post, error) {
	if m.GetPostFn != nil {
		return m.GetPostFn(ctx, key)
	}
	return []model.Post{}, nil
}

func (m *MockCache) SetUser(ctx context.Context, key string, user model.User, ttl time.Duration) error {
	if m.SetUserFn != nil {
		return m.SetUserFn(ctx, key, user, ttl)
	}
	return nil
}

func (m *MockCache) GetUser(ctx context.Context, key string) (model.User, error) {
	if m.GetUserFn != nil {
		return m.GetUserFn(ctx, key)
	}
	return model.User{}, nil
}

func (m *MockCache) SetFeeds(ctx context.Context, key string, feeds []model.Feed, ttl time.Duration) error {
	if m.SetFeedsFn != nil {
		return m.SetFeedsFn(ctx, key, feeds, ttl)
	}
	return nil
}

func (m *MockCache) GetFeeds(ctx context.Context, key string) ([]model.Feed, error) {
	if m.GetFeedsFn != nil {
		return m.GetFeedsFn(ctx, key)
	}
	return []model.Feed{}, nil
}

func (m *MockCache) Delete(ctx context.Context, key string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, key)
	}
	return nil
}
