package handler

import (
	"log/slog"
	"net/http"

	"FeedFlow/internal/httpapi"
)

func (h *Handler) PostFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type parameters struct {
		Name string `json:"name"`
		Url  string `json:"url"`
	}
	var params parameters

	defer r.Body.Close()
	if err := httpapi.DecodeJSON(w, r, &params); err != nil {
		httpapi.WriteDecodeError(w, err)
		return
	}

	savedFeed, err := h.storage.AddFeed(ctx, h.idGenerator(), params.Name, params.Url)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create feed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create feed")
		return
	}

	if err := h.cache.Delete(ctx, "feeds:all"); err != nil {
		slog.WarnContext(ctx, "failed to invalidate feeds cache", "feed_id", savedFeed.ID.String(), "error", err)
	}

	httpapi.WriteJSON(w, http.StatusOK, savedFeed)
}
