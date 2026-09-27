package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "FeedFlow/internal/httpapi"
	notification "FeedFlow/internal/notification/model"
	channelrepo "FeedFlow/internal/notification/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelStorageErrors(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	channelPath := "/v1/notification-channel/" + uuid.NewString()
	for _, failure := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", channelrepo.ErrChannelNotFound, 404, "channel_not_found"},
		{"conflict", channelrepo.ErrChannelConflict, 409, "channel_conflict"},
		{"unavailable", channelrepo.ErrUnavailable, 503, "service_unavailable"},
		{"unexpected", errors.New("SQL secret and private recipient"), 500, "internal_error"},
	} {
		for _, route := range []struct{ method, path, body string }{
			{"POST", "/v1/notification-channel", `{"type":"email","destination":"test@example.com"}`},
			{"GET", "/v1/notification-channel", ""},
			{"PATCH", channelPath, `{"enabled":false}`},
			{"DELETE", channelPath, ""},
		} {
			if failure.name == "not found" && route.method != "PATCH" && route.method != "DELETE" {
				continue
			}
			if failure.name == "conflict" && route.method != "POST" {
				continue
			}
			t.Run(failure.name+"/"+route.method, func(t *testing.T) {
				wrapped := fmt.Errorf("private dependency details: %w", failure.err)
				calls := 0
				store := &MockNotificationChannelStorage{
					CreateChannelFn: func(context.Context, notification.Channel) (notification.Channel, error) {
						calls++
						return notification.Channel{}, wrapped
					},
					GetChannelsFn: func(context.Context, uuid.UUID) ([]notification.Channel, error) { calls++; return nil, wrapped },
					SetChannelEnabledFn: func(context.Context, uuid.UUID, uuid.UUID, bool) (notification.Channel, error) {
						calls++
						return notification.Channel{}, wrapped
					},
					DeleteChannelFn: func(context.Context, uuid.UUID, uuid.UUID) error { calls++; return wrapped },
				}
				r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				r.Header.Set("Authorization", "Bearer "+token.AccessToken)
				w := httptest.NewRecorder()
				NewHandler(store).Router(verifier).ServeHTTP(w, r)
				assert.Equal(t, failure.status, w.Code)
				assert.Equal(t, 1, calls)
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				var response api.ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, failure.code, response.Error.Code)
				assert.NotContains(t, w.Body.String(), "private")
				assert.NotContains(t, w.Body.String(), "SQL")
			})
		}
	}
}

func TestInvalidChannelRequestsNeverCallStorage(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	store := &MockNotificationChannelStorage{
		CreateChannelFn: func(context.Context, notification.Channel) (notification.Channel, error) {
			t.Fatal("invalid create reached storage")
			return notification.Channel{}, nil
		},
		SetChannelEnabledFn: func(context.Context, uuid.UUID, uuid.UUID, bool) (notification.Channel, error) {
			t.Fatal("invalid patch reached storage")
			return notification.Channel{}, nil
		},
		DeleteChannelFn: func(context.Context, uuid.UUID, uuid.UUID) error {
			t.Fatal("invalid delete reached storage")
			return nil
		},
	}
	path := "/v1/notification-channel/" + uuid.NewString()
	for _, tt := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"malformed", "POST", "/v1/notification-channel", `{`, 400},
		{"null", "POST", "/v1/notification-channel", `null`, 400},
		{"client user ID", "POST", "/v1/notification-channel", `{"type":"email","destination":"a@b.c","user_id":"` + uuid.NewString() + `"}`, 400},
		{"client channel ID", "POST", "/v1/notification-channel", `{"type":"email","destination":"a@b.c","id":"` + uuid.NewString() + `"}`, 400},
		{"unsupported type", "POST", "/v1/notification-channel", `{"type":"webhook","destination":"a@b.c"}`, 400},
		{"empty destination", "POST", "/v1/notification-channel", `{"type":"email","destination":"  "}`, 400},
		{"long destination", "POST", "/v1/notification-channel", `{"type":"email","destination":"` + strings.Repeat("x", 513) + `"}`, 400},
		{"multiple values", "POST", "/v1/notification-channel", `{"type":"email","destination":"a@b.c"}{}`, 400},
		{"body limit", "POST", "/v1/notification-channel", `{"type":"email","destination":"` + strings.Repeat("x", int(api.MaxJSONBodyBytes)) + `"}`, 413},
		{"missing enabled", "PATCH", path, `{}`, 400},
		{"null enabled", "PATCH", path, `{"enabled":null}`, 400},
		{"wrong enabled type", "PATCH", path, `{"enabled":"false"}`, 400},
		{"patch user ID", "PATCH", path, `{"enabled":false,"user_id":"` + uuid.NewString() + `"}`, 400},
		{"invalid ID", "PATCH", "/v1/notification-channel/invalid", `{"enabled":false}`, 400},
		{"zero ID", "DELETE", "/v1/notification-channel/" + uuid.Nil.String(), "", 400},
		{"invalid delete ID", "DELETE", "/v1/notification-channel/invalid", "", 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			r.Header.Set("Authorization", "Bearer "+token.AccessToken)
			w := httptest.NewRecorder()
			NewHandler(store).Router(verifier).ServeHTTP(w, r)
			assert.Equal(t, tt.status, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		})
	}
}

func TestChannelRoutesRequireBearer(t *testing.T) {
	_, verifier := testTokens(t)
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/notification-channel"}, {"GET", "/v1/notification-channel"},
		{"PATCH", "/v1/notification-channel/" + uuid.NewString()}, {"DELETE", "/v1/notification-channel/" + uuid.NewString()},
	} {
		for _, header := range []string{"", "ApiKey key", "Bearer invalid"} {
			r := httptest.NewRequest(route.method, route.path, nil)
			r.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			NewHandler(nil).Router(verifier).ServeHTTP(w, r)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.NotEmpty(t, w.Header().Get("WWW-Authenticate"))
		}
	}
}

func TestChannelHandlersWithoutIdentityFailClosed(t *testing.T) {
	h := NewHandler(nil)
	for _, next := range []http.HandlerFunc{h.PostChannel, h.GetChannels, h.PatchChannel, h.DeleteChannel} {
		w := httptest.NewRecorder()
		next(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestChannelStorageMissingReturns503(t *testing.T) {
	issuer, verifier := testTokens(t)
	token, err := issuer.Issue(uuid.New())
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil)
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)
	w := httptest.NewRecorder()
	NewHandler(nil).Router(verifier).ServeHTTP(w, r)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
