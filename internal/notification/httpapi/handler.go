package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"FeedFlow/internal/auth"
	api "FeedFlow/internal/httpapi"
	notification "FeedFlow/internal/notification/model"
	channelrepo "FeedFlow/internal/notification/repository"

	"github.com/google/uuid"
)

type Handler struct {
	channels    channelrepo.ChannelRepository
	idGenerator func() uuid.UUID
}

func NewHandler(channels channelrepo.ChannelRepository) *Handler {
	return &Handler{channels: channels, idGenerator: uuid.New}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, verifier auth.TokenVerifier) {
	protected := auth.Middleware(verifier)
	mux.Handle("POST /v1/notification-channel", protected(http.HandlerFunc(h.PostChannel)))
	mux.Handle("GET /v1/notification-channel", protected(http.HandlerFunc(h.GetChannels)))
	mux.Handle("PATCH /v1/notification-channel/{channelID}", protected(http.HandlerFunc(h.PatchChannel)))
	mux.Handle("DELETE /v1/notification-channel/{channelID}", protected(http.HandlerFunc(h.DeleteChannel)))
}

func (h *Handler) Router(verifier auth.TokenVerifier) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, verifier)
	return mux
}

type channelResponse struct {
	ID          uuid.UUID                `json:"id"`
	Type        notification.ChannelType `json:"type"`
	Destination string                   `json:"destination"`
	Enabled     bool                     `json:"enabled"`
}

func responseFor(channel notification.Channel) channelResponse {
	return channelResponse{ID: channel.ID, Type: channel.Type, Destination: channel.Destination, Enabled: channel.Enabled}
}

func requestUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="feedflow"`)
		api.WriteError(w, http.StatusUnauthorized, "unauthorized", "Bearer access token is required")
	}
	return id, ok
}

func requestChannelID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("channelID"))
	if err != nil || id == uuid.Nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid channel ID")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) storageReady(w http.ResponseWriter) bool {
	if h.channels == nil {
		api.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "notification storage unavailable")
		return false
	}
	return true
}

func writeStorageError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, channelrepo.ErrChannelNotFound):
		api.WriteError(w, http.StatusNotFound, "channel_not_found", "notification channel not found")
	case errors.Is(err, channelrepo.ErrChannelConflict):
		api.WriteError(w, http.StatusConflict, "channel_conflict", "notification channel already exists")
	case errors.Is(err, channelrepo.ErrUnavailable):
		slog.ErrorContext(r.Context(), "notification storage unavailable", "operation", operation)
		api.WriteError(w, http.StatusServiceUnavailable, "service_unavailable", "notification storage unavailable")
	default:
		slog.ErrorContext(r.Context(), "notification operation failed", "operation", operation)
		api.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func (h *Handler) PostChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)
	var request struct {
		Type        notification.ChannelType `json:"type"`
		Destination string                   `json:"destination"`
	}
	if err := api.DecodeJSON(w, r, &request); err != nil {
		api.WriteDecodeError(w, err)
		return
	}
	if request.Type != notification.ChannelTelegram && request.Type != notification.ChannelEmail {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", "unsupported notification channel type")
		return
	}
	request.Destination = strings.TrimSpace(request.Destination)
	if request.Destination == "" || len(request.Destination) > 512 {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", "destination must contain between 1 and 512 bytes")
		return
	}
	if !h.storageReady(w) {
		return
	}
	channel, err := h.channels.CreateChannel(r.Context(), notification.Channel{
		ID: h.idGenerator(), UserID: userID, Type: request.Type, Destination: request.Destination,
	})
	if err != nil {
		writeStorageError(w, r, "create_channel", err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, responseFor(channel))
}

func (h *Handler) GetChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok || !h.storageReady(w) {
		return
	}
	channels, err := h.channels.GetChannels(r.Context(), userID)
	if err != nil {
		writeStorageError(w, r, "get_channels", err)
		return
	}
	response := make([]channelResponse, 0, len(channels))
	for _, channel := range channels {
		response = append(response, responseFor(channel))
	}
	api.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) PatchChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	channelID, ok := requestChannelID(w, r)
	if !ok {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := api.DecodeJSON(w, r, &request); err != nil {
		api.WriteDecodeError(w, err)
		return
	}
	if request.Enabled == nil {
		api.WriteError(w, http.StatusBadRequest, "invalid_request", "enabled field is required")
		return
	}
	if !h.storageReady(w) {
		return
	}
	channel, err := h.channels.SetChannelEnabled(r.Context(), userID, channelID, *request.Enabled)
	if err != nil {
		writeStorageError(w, r, "update_channel", err)
		return
	}
	api.WriteJSON(w, http.StatusOK, responseFor(channel))
}

func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	channelID, ok := requestChannelID(w, r)
	if !ok || !h.storageReady(w) {
		return
	}
	if err := h.channels.DeleteChannel(r.Context(), userID, channelID); err != nil {
		writeStorageError(w, r, "delete_channel", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
