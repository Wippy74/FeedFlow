package handler

import (
	"context"
	"net/http"
	"strings"
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
		vals := strings.Split(authHeader, " ")
		if len(vals) != 2 && vals[0] != "ApiKey" {
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}
		apiKey := vals[1]

		user, err := h.storage.GetUserByApiKey(r.Context(), apiKey)
		if err != nil {
			http.Error(w, "Invalid Api key", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}
