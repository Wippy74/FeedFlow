package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/httpapi"
	"FeedFlow/internal/model"

	"github.com/redis/go-redis/v9"
)

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow"`)
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "Bearer access token is required")
		return
	}
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 10
	offset := 0

	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
	}
	if offsetStr != "" {
		var err error
		offset, err = strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
	}

	cacheKey := fmt.Sprintf("posts:user:%s:limit:%d:offset:%d", userID, limit, offset)

	cachedPost, err := h.cache.GetPost(ctx, cacheKey)
	if err == nil {
		w.Header().Set("X-Cache", "HIT")
		httpapi.WriteJSON(w, http.StatusOK, cachedPost)
		return
	} else if !errors.Is(err, redis.Nil) {
		slog.WarnContext(ctx, "failed to get posts from cache", "user_id", userID.String(), "error", err)
	}

	posts, err := h.storage.GetPosts(r.Context(), userID, limit, offset)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get posts", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get posts")
		return
	}
	if posts == nil {
		posts = []model.Post{}
	}

	h.runInBackground(func(bgCtx context.Context) {
		if err := h.cache.SetPost(bgCtx, cacheKey, posts, 1*time.Minute); err != nil && bgCtx.Err() == nil {
			slog.WarnContext(bgCtx, "failed to cache posts", "user_id", userID.String(), "error", err)
		}
	})
	w.Header().Set("X-Cache", "MISS")
	httpapi.WriteJSON(w, http.StatusOK, posts)
}
