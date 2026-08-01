package handler

import (
	"NewsAggregator/internal/model"
	"encoding/json"
	"net/http"
	"strconv"
)

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(model.User)
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 10
	offset := 0

	if limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}
	if offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	feeds, err := h.storage.GetPosts(r.Context(), user.ID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(feeds)
}
