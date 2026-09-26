package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) PostToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	apiKey, ok := authorizationCredential(r, "ApiKey")
	if !ok {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="feedflow"`)
		http.Error(w, "API key is required", http.StatusUnauthorized)
		return
	}
	if h.tokenIssuer == nil {
		http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	user, err := h.storage.GetUserByApiKey(r.Context(), apiKey)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && user.ID == uuid.Nil) {
		w.Header().Set("WWW-Authenticate", `ApiKey realm="feedflow"`)
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	token, err := h.tokenIssuer.Issue(user.ID)
	if err != nil {
		http.Error(w, "could not issue access token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(token)
}
