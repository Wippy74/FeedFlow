package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"FeedFlow/internal/httpapi"
	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	defaultFeedsPageSize = 50
	maxFeedsPageSize     = 100
	firstFeedsPageKey    = "feeds:first:50"
)

func (h *Handler) GetAllFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := defaultFeedsPageSize
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxFeedsPageSize {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	var after *uuid.UUID
	if value := r.URL.Query().Get("after"); value != "" {
		parsed, err := uuid.Parse(value)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "after must be a UUID")
			return
		}
		after = &parsed
	}

	cacheFirstPage := after == nil && limit == defaultFeedsPageSize
	if cacheFirstPage {
		feeds, err := h.cache.GetFeeds(ctx, firstFeedsPageKey)
		if err == nil {
			writeFeedsPage(w, feeds, limit, "HIT")
			return
		} else if !errors.Is(err, redis.Nil) {
			slog.WarnContext(ctx, "failed to get feeds from cache", "error", err)
		}
	}

	feeds, err := h.storage.GetFeedsPage(ctx, after, limit+1)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get feeds", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get feeds")
		return
	}
	if feeds == nil {
		feeds = []model.Feed{}
	}

	if cacheFirstPage {
		h.runInBackground(func(bgCtx context.Context) {
			if err := h.cache.SetFeeds(bgCtx, firstFeedsPageKey, feeds, time.Minute); err != nil && bgCtx.Err() == nil {
				slog.WarnContext(bgCtx, "failed to cache feeds", "error", err)
			}
		})
	}
	writeFeedsPage(w, feeds, limit, "MISS")
}

func writeFeedsPage(w http.ResponseWriter, feeds []model.Feed, limit int, cacheStatus string) {
	if feeds == nil {
		feeds = []model.Feed{}
	}
	if len(feeds) > limit {
		feeds = feeds[:limit]
		w.Header().Set("X-Next-Cursor", feeds[len(feeds)-1].ID.String())
	}
	w.Header().Set("X-Cache", cacheStatus)
	httpapi.WriteJSON(w, http.StatusOK, feeds)
}
