package handler

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) GetAllFeeds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	allFeeds, err := h.storage.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(allFeeds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
