package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"FeedFlow/internal/model"

	"github.com/redis/go-redis/v9"
)

func (h *Handler) GetAllFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cacheKey := "feeds:all"
	w.Header().Set("Content-Type", "application/json")

	allFeeds, err := h.cache.GetFeeds(ctx, cacheKey)
	if err == nil {
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(allFeeds); err != nil {
			slog.ErrorContext(ctx, "failed to encode cached feeds", "error", err)
		}
		return
	} else if !errors.Is(err, redis.Nil) {
		slog.WarnContext(ctx, "failed to get feeds from cache", "error", err)
	}

	allFeeds, err = h.storage.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(allFeeds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
