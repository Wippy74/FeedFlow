package handler

import (
	"io"
	"log/slog"
	"net/http"

	"FeedFlow/internal/httpapi"
)

func (h *Handler) PostUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	ctx := r.Context()
	type params struct {
		Name string `json:"name"`
	}

	var parameters params

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)
	if err := httpapi.DecodeJSON(w, r, &parameters); err != nil {
		httpapi.WriteDecodeError(w, err)
		return
	}

	apiKey, err := h.apiKeyGenerator()
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to generate API key")
		return
	}

	savedUser, err := h.storage.SaveUser(ctx, h.idGenerator(), parameters.Name, apiKey)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create user")
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create user")
		return
	}

	httpapi.WriteJSON(w, http.StatusCreated, savedUser)
}
