package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

func (h *Handler) PostUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type params struct {
		ID     uuid.UUID `json:"id"`
		Name   string    `json:"name"`
		ApiKey string    `json:"apiKey"`
	}

	var parameters params

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&parameters); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	savedUser, err := h.storage.SaveUser(ctx, parameters.ID, parameters.Name, parameters.ApiKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(savedUser); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
