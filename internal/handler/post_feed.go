package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

func (h *Handler) PostFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type parameters struct {
		Name string `json:"name"`
		Url  string `json:"url"`
	}
	var params parameters

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	savedFeed, err := h.storage.AddFeed(ctx, h.idGenerator(), params.Name, params.Url)
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
