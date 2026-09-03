package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostUserGeneratesIDAndAPIKeyOnServer(t *testing.T) {
	serverID := uuid.New()
	const serverAPIKey = "na_server-generated-key"

	h := NewHandler(&MockStorage{
		SaveUserFn: func(_ context.Context, id uuid.UUID, name, apiKey string) (model.User, error) {
			assert.Equal(t, serverID, id)
			assert.Equal(t, "Test User", name)
			assert.Equal(t, serverAPIKey, apiKey)
			return model.User{ID: id, Name: name, ApiKey: apiKey}, nil
		},
	}, &MockCache{})
	h.idGenerator = func() uuid.UUID { return serverID }
	h.apiKeyGenerator = func() (string, error) { return serverAPIKey, nil }

	body := bytes.NewBufferString(`{"name":"Test User"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/users", body)
	rr := httptest.NewRecorder()

	h.PostUser(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var response model.User
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, serverID, response.ID)
	assert.Equal(t, serverAPIKey, response.ApiKey)
}

func TestPostUserRejectsClientGeneratedCredentials(t *testing.T) {
	tests := []string{
		`{"name":"Test User","id":"` + uuid.New().String() + `"}`,
		`{"name":"Test User","apiKey":"client-controlled-key"}`,
	}

	for _, body := range tests {
		h := NewHandler(&MockStorage{
			SaveUserFn: func(context.Context, uuid.UUID, string, string) (model.User, error) {
				t.Fatal("storage must not be called when request contains server-managed fields")
				return model.User{}, nil
			},
		}, &MockCache{})
		req := httptest.NewRequest(http.MethodPost, "/v1/users", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()

		h.PostUser(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	}
}

func TestPostUserRejectsInvalidPayload(t *testing.T) {
	h := NewHandler(&MockStorage{
		SaveUserFn: func(context.Context, uuid.UUID, string, string) (model.User, error) {
			t.Fatal("storage must not be called for an invalid payload")
			return model.User{}, nil
		},
	}, &MockCache{})

	req := httptest.NewRequest(http.MethodPost, "/v1/users", bytes.NewBufferString("invalid json"))
	rr := httptest.NewRecorder()

	h.PostUser(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostUserHandlesAPIKeyGenerationFailure(t *testing.T) {
	h := NewHandler(&MockStorage{
		SaveUserFn: func(context.Context, uuid.UUID, string, string) (model.User, error) {
			t.Fatal("storage must not be called when API key generation fails")
			return model.User{}, nil
		},
	}, &MockCache{})
	h.apiKeyGenerator = func() (string, error) { return "", errors.New("random source unavailable") }

	req := httptest.NewRequest(http.MethodPost, "/v1/users", bytes.NewBufferString(`{"name":"Test User"}`))
	rr := httptest.NewRecorder()

	h.PostUser(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPostUserHandlesStorageFailure(t *testing.T) {
	h := NewHandler(&MockStorage{
		SaveUserFn: func(context.Context, uuid.UUID, string, string) (model.User, error) {
			return model.User{}, errors.New("db error")
		},
	}, &MockCache{})

	req := httptest.NewRequest(http.MethodPost, "/v1/users", bytes.NewBufferString(`{"name":"Test User"}`))
	rr := httptest.NewRecorder()

	h.PostUser(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}
