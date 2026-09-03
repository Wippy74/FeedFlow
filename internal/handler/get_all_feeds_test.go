package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllFeeds(t *testing.T) {
	cachedFeeds := []model.Feed{
		{ID: uuid.New(), Name: "Cached Feed", Url: "http://example.com/cached"},
	}
	dbFeeds := []model.Feed{
		{ID: uuid.New(), Name: "DB Feed", Url: "http://example.com/db"},
	}

	tests := []struct {
		name                string
		mockGetFeedsFn      func(ctx context.Context, key string) ([]model.Feed, error)
		mockGetAllFeedsFn   func(ctx context.Context) ([]model.Feed, error)
		expectedStatus      int
		expectedCacheHeader string
		expectedFeedName    string
		expectCacheSetCall  bool
	}{
		{
			name: "Cache Hit",
			mockGetFeedsFn: func(ctx context.Context, key string) ([]model.Feed, error) {
				return cachedFeeds, nil
			},
			mockGetAllFeedsFn:   nil,
			expectedStatus:      http.StatusOK,
			expectedCacheHeader: "HIT",
			expectedFeedName:    "Cached Feed",
			expectCacheSetCall:  false,
		},
		{
			name: "Cache Miss, DB Success",
			mockGetFeedsFn: func(ctx context.Context, key string) ([]model.Feed, error) {
				return nil, redis.Nil
			},
			mockGetAllFeedsFn: func(ctx context.Context) ([]model.Feed, error) {
				return dbFeeds, nil
			},
			expectedStatus:      http.StatusOK,
			expectedCacheHeader: "MISS",
			expectedFeedName:    "DB Feed",
			expectCacheSetCall:  true,
		},
		{
			name: "Cache Miss, DB Error",
			mockGetFeedsFn: func(ctx context.Context, key string) ([]model.Feed, error) {
				return nil, redis.Nil
			},
			mockGetAllFeedsFn: func(ctx context.Context) ([]model.Feed, error) {
				return nil, errors.New("db error")
			},
			expectedStatus:      http.StatusInternalServerError,
			expectedCacheHeader: "",
			expectCacheSetCall:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheSetCalled := make(chan bool, 1)

			mockStorage := &MockStorage{
				GetAllFeedsFn: tt.mockGetAllFeedsFn,
			}
			mockCache := &MockCache{
				GetFeedsFn: tt.mockGetFeedsFn,
				SetFeedsFn: func(ctx context.Context, key string, feeds []model.Feed, ttl time.Duration) error {
					cacheSetCalled <- true
					return nil
				},
			}
			h := NewHandler(mockStorage, mockCache)

			req, err := http.NewRequest("GET", "/v1/feeds", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			h.GetAllFeeds(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedCacheHeader != "" {
				assert.Equal(t, tt.expectedCacheHeader, rr.Header().Get("X-Cache"))
			}

			if tt.expectedStatus == http.StatusOK {
				var responseFeeds []model.Feed
				err = json.Unmarshal(rr.Body.Bytes(), &responseFeeds)
				require.NoError(t, err)
				assert.Len(t, responseFeeds, 1)
				assert.Equal(t, tt.expectedFeedName, responseFeeds[0].Name)
			}

			if tt.expectCacheSetCall {
				select {
				case <-cacheSetCalled:
				case <-time.After(1 * time.Second):
					t.Fatal("Cache SetFeeds was not called in time")
				}
			}
		})
	}
}
