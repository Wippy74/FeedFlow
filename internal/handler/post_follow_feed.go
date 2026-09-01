package handler

import (
	"NewsAggregator/internal/model"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

func (h *Handler) PostFollowFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type parameters struct {
		FeedID uuid.UUID `json:"feedId"`
	}

	var params parameters
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	user, ok := ctx.Value(userContextKey).(model.User)
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	err := h.storage.FollowFeed(ctx, user.ID, params.FeedID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
}
