package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func (h *Handler) PostFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type parameters struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
		Url  string    `json:"url"`
	}
	var params parameters

	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	savedFeed, err := h.storage.AddFeed(ctx, params.ID, params.Name, params.Url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go func() {
		if err := h.cache.Delete(context.Background(), "feeds:all"); err != nil {
			log.Printf("Failed to delete cache: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(savedFeed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
