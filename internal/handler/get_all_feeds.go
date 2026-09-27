package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"FeedFlow/internal/httpapi"
	"FeedFlow/internal/model"

	"github.com/redis/go-redis/v9"
)

func (h *Handler) GetAllFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cacheKey := "feeds:all"

	allFeeds, err := h.cache.GetFeeds(ctx, cacheKey)
	if err == nil {
		w.Header().Set("X-Cache", "HIT")
		httpapi.WriteJSON(w, http.StatusOK, allFeeds)
		return
	} else if !errors.Is(err, redis.Nil) {
		slog.WarnContext(ctx, "failed to get feeds from cache", "error", err)
	}

	allFeeds, err = h.storage.GetAllFeeds(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get feeds", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get feeds")
		return
	}
	if allFeeds == nil {
		allFeeds = []model.Feed{}
	}

	h.runInBackground(func(bgCtx context.Context) {
		if err := h.cache.SetFeeds(bgCtx, cacheKey, allFeeds, 1*time.Hour); err != nil && bgCtx.Err() == nil {
			slog.WarnContext(bgCtx, "failed to cache feeds", "error", err)
		}
	})

	w.Header().Set("X-Cache", "MISS")
	httpapi.WriteJSON(w, http.StatusOK, allFeeds)
}
