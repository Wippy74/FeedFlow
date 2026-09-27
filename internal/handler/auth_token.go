package handler

import (
	"errors"
	"net/http"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/httpapi"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) PostToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	apiKey, ok := auth.AuthorizationCredential(r, "ApiKey")
	if !ok {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="feedflow"`)
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "API key is required")
		return
	}
	if h.tokenIssuer == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "authentication_unavailable", "authentication unavailable")
		return
	}
	user, err := h.storage.GetUserByApiKey(r.Context(), apiKey)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.ID == uuid.Nil) {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="feedflow"`)
		httpapi.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "invalid API key")
		return
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "authentication_unavailable", "authentication unavailable")
		return
	}
	token, err := h.tokenIssuer.Issue(user.ID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not issue access token")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, token)
}
