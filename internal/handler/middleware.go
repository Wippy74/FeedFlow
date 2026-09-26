package handler

import (
	"context"
	"net/http"
	"strings"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
)

type contextKey string

const userContextKey = contextKey("user")

func authorizationCredential(r *http.Request, scheme string) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], scheme) || len(parts[1]) > 8192 {
		return "", false
	}
	return parts[1], true
}

func (h *Handler) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, ok := authorizationCredential(r, "Bearer")
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow"`)
			http.Error(w, "Bearer access token is required", http.StatusUnauthorized)
			return
		}
		if h.tokenVerifier == nil {
			http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := h.tokenVerifier.Verify(value)
		if err != nil || userID == uuid.Nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow", error="invalid_token"`)
			http.Error(w, "invalid access token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, model.User{ID: userID})
		next(w, r.WithContext(ctx))
	}
}
