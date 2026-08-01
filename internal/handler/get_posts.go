package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var userID uuid.UUID

	if err := json.NewDecoder(r.Body).Decode(&userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	feeds, err := h.storage.GetPosts(ctx, userID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(feeds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
