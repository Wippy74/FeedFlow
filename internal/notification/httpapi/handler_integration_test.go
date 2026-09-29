package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	notificationpostgres "FeedFlow/internal/notification/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("FEEDFLOW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FEEDFLOW_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"http_channels_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		assert.NoError(t, err)
	})
	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, name := range []string{"001_notification_channels", "002_notification_inbox",
		"003_notification_deliveries", "004_notification_dispatch_outbox"} {
		migration, err := os.ReadFile("../migrations/sql/" + name + ".up.sql")
		require.NoError(t, err)
		_, err = pool.Exec(ctx, string(migration), pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, err)
	}
	return pool
}

func TestChannelAPIIntegrationOwnershipAndErrors(t *testing.T) {
	pool := integrationPool(t)
	ownerID, otherID := uuid.New(), uuid.New()
	issuer, verifier := testTokens(t)
	ownerToken, err := issuer.Issue(ownerID)
	require.NoError(t, err)
	otherToken, err := issuer.Issue(otherID)
	require.NoError(t, err)
	router := NewHandler(notificationpostgres.NewRepository(pool)).Router(verifier)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		// A forged identity header must not affect repository scoping.
		r.Header.Set("X-User-ID", ownerID.String())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	const base = "/v1/notification-channel"
	const create = `{"type":"email","destination":" shared@example.com "}`
	w := request("POST", base, create, ownerToken.AccessToken)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var ownerChannel channelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ownerChannel))
	assert.True(t, ownerChannel.Enabled)
	assert.Equal(t, "shared@example.com", ownerChannel.Destination)
	ownerPath := base + "/" + ownerChannel.ID.String()

	w = request("POST", base, create, ownerToken.AccessToken)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.JSONEq(t, `{"error":{"code":"channel_conflict","message":"notification channel already exists"}}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), "shared@example.com")

	w = request("GET", base, "", otherToken.AccessToken)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
	w = request("POST", base, create, otherToken.AccessToken)
	require.Equal(t, http.StatusCreated, w.Code, "same destination is allowed for a different user")
	var otherChannel channelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &otherChannel))

	for _, method := range []string{"PATCH", "DELETE"} {
		foreign := request(method, ownerPath, `{"enabled":false}`, otherToken.AccessToken)
		missing := request(method, base+"/"+uuid.NewString(), `{"enabled":false}`, otherToken.AccessToken)
		assert.Equal(t, http.StatusNotFound, foreign.Code)
		assert.Equal(t, http.StatusNotFound, missing.Code)
		assert.Equal(t, foreign.Body.String(), missing.Body.String(), "do not disclose whether a foreign channel exists")
	}
	w = request("GET", base, "", ownerToken.AccessToken)
	var list []channelResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, ownerChannel.ID, list[0].ID)
	assert.True(t, list[0].Enabled, "foreign mutation did not change the owner channel")
	w = request("GET", base, "", otherToken.AccessToken)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, otherChannel.ID, list[0].ID)

	w = request("PATCH", ownerPath, `{"enabled":false}`, ownerToken.AccessToken)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ownerChannel))
	assert.False(t, ownerChannel.Enabled)
	w = request("DELETE", ownerPath, "", ownerToken.AccessToken)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
	w = request("DELETE", ownerPath, "", ownerToken.AccessToken)
	assert.Equal(t, http.StatusNotFound, w.Code)

	pool.Close()
	for _, route := range []struct{ method, path, body string }{
		{"POST", base, create}, {"GET", base, ""},
		{"PATCH", ownerPath, `{"enabled":true}`}, {"DELETE", ownerPath, ""},
	} {
		w = request(route.method, route.path, route.body, ownerToken.AccessToken)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, route.method)
		assert.NotContains(t, w.Body.String(), "closed pool")
	}
}
