package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"FeedFlow/internal/model"
	"FeedFlow/internal/notification/model"

	"github.com/google/uuid"
)

type NotificationChannelStorage interface {
	CreateChannel(ctx context.Context, channel notification.Channel) (notification.Channel, error)
	GetChannels(ctx context.Context, userID uuid.UUID) ([]notification.Channel, error)
	SetChannelEnabled(ctx context.Context, userID uuid.UUID, channelID uuid.UUID, enabled bool) (notification.Channel, error)
	DeleteChannel(ctx context.Context, userID, channelID uuid.UUID) error
}
type notificationChannelResponse struct {
	ID          uuid.UUID                `json:"id"`
	Type        notification.ChannelType `json:"type"`
	Destination string                   `json:"destination"`
	Enabled     bool                     `json:"enabled"`
}

func newNotificationChannelResponse(channel notification.Channel) notificationChannelResponse {
	return notificationChannelResponse{
		ID:          channel.ID,
		Type:        channel.Type,
		Destination: channel.Destination,
		Enabled:     channel.Enabled,
	}
}

func decodeNotificationJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("could not decode JSON: %w", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request body must contain one JSON object")
	}
	return nil
}

func notificationUserFromRequest(r *http.Request) (model.User, bool) {
	user, ok := r.Context().Value(userContextKey).(model.User)
	return user, ok
}

func writeNotificationJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("failed to encode notification response", "error", err)
	}
}

func validNotificationChannelType(channelType notification.ChannelType) bool {
	switch channelType {
	case notification.ChannelTelegram,
		notification.ChannelEmail:
		return true
	default:
		return false
	}
}

func (h *Handler) PostNotificationChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := notificationUserFromRequest(r)
	if !ok {
		http.Error(w, "user not found in context", http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = r.Body.Close()
	}()

	var request struct {
		Type        notification.ChannelType `json:"type"`
		Destination string                   `json:"destination"`
	}
	if err := decodeNotificationJSON(r, &request); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	request.Destination = strings.TrimSpace(request.Destination)

	if !validNotificationChannelType(request.Type) {
		http.Error(w, "unsupported notification channel type", http.StatusBadRequest)
		return
	}
	if request.Destination == "" {
		http.Error(w, "destination must not be empty", http.StatusBadRequest)
		return
	}

	if len(request.Destination) > 512 {
		http.Error(w, "destination is too long", http.StatusBadRequest)
		return
	}

	channel, err := h.notificationChannels.CreateChannel(
		r.Context(),
		notification.Channel{
			ID:          h.idGenerator(),
			UserID:      user.ID,
			Type:        request.Type,
			Destination: request.Destination,
		},
	)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to create notification channel",
			"user_id", user.ID.String(), "channel_type", request.Type, "error", err,
		)
		http.Error(w, "failed to create notification channel", http.StatusInternalServerError)
		return
	}

	writeNotificationJSON(w, http.StatusCreated, newNotificationChannelResponse(channel))
}

func (h *Handler) GetNotificationChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := notificationUserFromRequest(r)
	if !ok {
		http.Error(w, "user not found in context", http.StatusInternalServerError)
		return
	}

	channels, err := h.notificationChannels.GetChannels(r.Context(), user.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to get notification channels", "user_id", user.ID.String(), "error", err)
		http.Error(w, "failed to get notification channels", http.StatusInternalServerError)
		return
	}

	response := make([]notificationChannelResponse, 0, len(channels))
	for _, channel := range channels {
		response = append(response, newNotificationChannelResponse(channel))
	}

	writeNotificationJSON(w, http.StatusOK, response)
}

func (h *Handler) PatchNotificationChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := notificationUserFromRequest(r)
	if !ok {
		http.Error(w, "user not found in context", http.StatusInternalServerError)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("channelID"))
	if err != nil {
		http.Error(w, "invalid request path", http.StatusBadRequest)
		return
	}
	defer func() {
		_ = r.Body.Close()
	}()

	var request struct {
		Enabled *bool `json:"enabled"`
	}

	if err := decodeNotificationJSON(r, &request); err != nil {
		http.Error(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	if request.Enabled == nil {
		http.Error(w, "enabled field is required", http.StatusBadRequest)
		return
	}

	channel, err := h.notificationChannels.SetChannelEnabled(r.Context(), user.ID, channelID, *request.Enabled)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to update notification channel", "user_id", user.ID.String(), "channel_id", channelID.String(), "error", err)
		http.Error(w, "failed to update notification channel", http.StatusInternalServerError)
		return
	}

	writeNotificationJSON(w, http.StatusOK, newNotificationChannelResponse(channel))
}

func (h *Handler) DeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := notificationUserFromRequest(r)
	if !ok {
		http.Error(w, "user not found in context", http.StatusInternalServerError)
		return
	}

	channelID, err := uuid.Parse(r.PathValue("channelID"))
	if err != nil {
		http.Error(w, "invalid channel ID", http.StatusBadRequest)
		return
	}

	if err := h.notificationChannels.DeleteChannel(r.Context(), user.ID, channelID); err != nil {
		slog.ErrorContext(r.Context(), "failed to delete notification channel", "user_id", user.ID.String(), "channel_id", channelID.String(), "error", err)
		http.Error(w, "failed to delete notification channel", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
