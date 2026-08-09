package cache

import (
	"NewsAggregator/internal/model"
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
	client := redis.NewClient(&redis.Options{Addr: addr})
	return &RedisCache{client: client}
}

func (c *RedisCache) Set(ctx context.Context, key string, posts []model.Post, ttl time.Duration) error {
	data, err := json.Marshal(posts)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]model.Post, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var posts []model.Post
	if err := json.Unmarshal([]byte(val), &posts); err != nil {
		return nil, err
	}

	return posts, nil
}
