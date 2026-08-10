package handler

import (
	"NewsAggregator/internal/model"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

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
		json.NewEncoder(w).Encode(allFeeds)
		return
	} else if !errors.Is(err, redis.Nil) {
		log.Printf("Error getting all feeds: %v", err)
	}

	allFeeds, err = h.storage.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if allFeeds == nil {
		allFeeds = []model.Feed{}
	}

	go func(key string, feeds []model.Feed) {
		bgCtx := context.Background()
		if err := h.cache.SetFeeds(bgCtx, key, feeds, 1*time.Hour); err != nil {
			log.Printf("Error setting feeds: %v", err)
		}
	}(cacheKey, allFeeds)

	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(allFeeds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
