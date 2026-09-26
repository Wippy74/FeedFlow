package auth

import (
	"context"
	"net/http"
	"strings"

	"FeedFlow/internal/httpapi"

	"github.com/google/uuid"
)

type TokenVerifier interface {
	Verify(string) (uuid.UUID, error)
}

type userIDContextKey struct{}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func AuthorizationCredential(r *http.Request, scheme string) (string, bool) {
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

// Middleware authenticates locally and fails closed. It never needs a user
// repository, cache, notification settings or the token issuer's private key.
func Middleware(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value, ok := AuthorizationCredential(r, "Bearer")
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow"`)
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "Bearer access token is required")
				return
			}
			if verifier == nil {
				httpapi.WriteError(w, http.StatusServiceUnavailable, "authentication_unavailable", "authentication unavailable")
				return
			}
			userID, err := verifier.Verify(value)
			if err != nil || userID == uuid.Nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow", error="invalid_token"`)
				httpapi.WriteError(w, http.StatusUnauthorized, "invalid_token", "invalid access token")
				return
			}
			ctx := context.WithValue(r.Context(), userIDContextKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
