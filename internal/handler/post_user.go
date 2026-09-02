package handler

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) PostUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type params struct {
		Name string `json:"name"`
	}

	var parameters params

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parameters); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	apiKey, err := h.apiKeyGenerator()
	if err != nil {
		http.Error(w, "failed to generate API key", http.StatusInternalServerError)
		return
	}

	savedUser, err := h.storage.SaveUser(ctx, h.idGenerator(), parameters.Name, apiKey)
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
