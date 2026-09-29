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
		mockGetFeedsPageFn  func(ctx context.Context, after *uuid.UUID, limit int) ([]model.Feed, error)
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
			mockGetFeedsPageFn:  nil,
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
			mockGetFeedsPageFn: func(ctx context.Context, after *uuid.UUID, limit int) ([]model.Feed, error) {
				assert.Nil(t, after)
				assert.Equal(t, defaultFeedsPageSize+1, limit)
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
			mockGetFeedsPageFn: func(ctx context.Context, after *uuid.UUID, limit int) ([]model.Feed, error) {
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
				GetFeedsPageFn: tt.mockGetFeedsPageFn,
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

func TestGetAllFeedsPagination(t *testing.T) {
	firstID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	secondID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	queryCount := 0
	h := NewHandler(&MockStorage{
		GetFeedsPageFn: func(_ context.Context, after *uuid.UUID, limit int) ([]model.Feed, error) {
			queryCount++
			assert.Equal(t, 2, limit)
			if after == nil {
				return []model.Feed{{ID: firstID}, {ID: secondID}}, nil
			}
			assert.Equal(t, firstID, *after)
			return []model.Feed{{ID: secondID}}, nil
		},
	}, &MockCache{
		GetFeedsFn: func(context.Context, string) ([]model.Feed, error) {
			t.Fatal("custom page size and cursor must bypass first-page cache")
			return nil, nil
		},
	})

	first := httptest.NewRecorder()
	h.GetAllFeeds(first, httptest.NewRequest(http.MethodGet, "/v1/feeds?limit=1", nil))
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, firstID.String(), first.Header().Get("X-Next-Cursor"))
	var feeds []model.Feed
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &feeds))
	require.Len(t, feeds, 1)
	require.Equal(t, firstID, feeds[0].ID)

	second := httptest.NewRecorder()
	h.GetAllFeeds(second, httptest.NewRequest(http.MethodGet, "/v1/feeds?limit=1&after="+firstID.String(), nil))
	require.Equal(t, http.StatusOK, second.Code)
	require.Empty(t, second.Header().Get("X-Next-Cursor"))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &feeds))
	require.Len(t, feeds, 1)
	require.Equal(t, secondID, feeds[0].ID)
	require.Equal(t, 2, queryCount)
}

func TestGetAllFeedsRejectsInvalidPagination(t *testing.T) {
	h := NewHandler(&MockStorage{
		GetFeedsPageFn: func(context.Context, *uuid.UUID, int) ([]model.Feed, error) {
			t.Fatal("invalid pagination must not reach storage")
			return nil, nil
		},
	}, &MockCache{})
	for _, query := range []string{"limit=0", "limit=101", "limit=abc", "after=invalid"} {
		t.Run(query, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.GetAllFeeds(rr, httptest.NewRequest(http.MethodGet, "/v1/feeds?"+query, nil))
			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}
