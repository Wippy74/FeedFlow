package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

func (h *Handler) PostFollowFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type parameters struct {
		UserID uuid.UUID `json:"userId"`
		FeedID uuid.UUID `json:"feedId"`
	}

	var params parameters
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	err := h.storage.FollowFeed(ctx, params.UserID, params.FeedID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
}
