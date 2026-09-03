package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"FeedFlow/internal/model"

	"github.com/redis/go-redis/v9"
)

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := ctx.Value(userContextKey).(model.User)
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 10
	offset := 0

	if limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}
	if offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	cacheKey := fmt.Sprintf("posts:user:%s:limit:%d:offset:%d", user.ID, limit, offset)
	w.Header().Set("Content-Type", "application/json")

	cachedPost, err := h.cache.GetPost(ctx, cacheKey)
	if err == nil {
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(cachedPost); err != nil {
			slog.ErrorContext(ctx, "failed to encode cached posts", "error", err)
		}
		return
	} else if !errors.Is(err, redis.Nil) {
		slog.WarnContext(ctx, "failed to get posts from cache", "user_id", user.ID.String(), "error", err)
	}

	posts, err := h.storage.GetPosts(r.Context(), user.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if posts == nil {
		posts = []model.Post{}
	}

	h.runInBackground(func(bgCtx context.Context) {
		if err := h.cache.SetPost(bgCtx, cacheKey, posts, 1*time.Minute); err != nil && bgCtx.Err() == nil {
			slog.WarnContext(bgCtx, "failed to cache posts", "user_id", user.ID.String(), "error", err)
		}
	})
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(posts); err != nil {
		slog.ErrorContext(ctx, "failed to encode posts", "user_id", user.ID.String(), "error", err)
	}
}
