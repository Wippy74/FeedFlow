package handler

import (
	"io"
	"log/slog"
	"net/http"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/httpapi"

	"github.com/google/uuid"
)

func (h *Handler) PostFollowFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow"`)
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "Bearer access token is required")
		return
	}
	type parameters struct {
		FeedID uuid.UUID `json:"feedId"`
	}

	var params parameters
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)
	if err := httpapi.DecodeJSON(w, r, &params); err != nil {
		httpapi.WriteDecodeError(w, err)
		return
	}
	if params.FeedID == uuid.Nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "feedId must be a non-zero UUID")
		return
	}

	err := h.storage.FollowFeed(ctx, userID, params.FeedID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to follow feed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to follow feed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
}
