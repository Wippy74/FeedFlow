package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"NewsAggregator/internal/model"

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
			log.Printf("error encoding cached posts: %v", err)
		}
		return
	} else if !errors.Is(err, redis.Nil) {
		log.Printf("error during cache get: %v", err)
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
			log.Printf("error during cache set: %v", err)
		}
	})
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(posts); err != nil {
		log.Printf("error encoding posts: %v", err)
	}
}
