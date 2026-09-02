package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type contextKey string

const userContextKey = contextKey("user")

func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Empty Authorization header", http.StatusUnauthorized)
			return
		}
		vals := strings.Fields(authHeader)
		if len(vals) != 2 || vals[0] != "ApiKey" || vals[1] == "" {
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}
		apiKey := vals[1]

		ctx := r.Context()
		cacheKey := fmt.Sprintf("auth:apikey:%s", apiKey)
		user, err := h.cache.GetUser(ctx, cacheKey)
		if err == nil {
			ctx = context.WithValue(ctx, userContextKey, user)
			next(w, r.WithContext(ctx))
			return
		} else if !errors.Is(err, redis.Nil) {
			log.Printf("Error getting user from cache: %v", err)
		}

		user, err = h.storage.GetUserByApiKey(ctx, apiKey)
		if err != nil {
			http.Error(w, "Invalid Api key", http.StatusUnauthorized)
			return
		}

		h.runInBackground(func(bgCtx context.Context) {
			if err := h.cache.SetUser(bgCtx, cacheKey, user, 15*time.Minute); err != nil && bgCtx.Err() == nil {
				log.Printf("Error setting user to cache: %v", err)
			}
		})

		ctx = context.WithValue(ctx, userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}
