package gateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesAndForwardsRequests(t *testing.T) {
	upstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read upstream request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("X-Upstream", name)
			w.Header().Set("X-Seen-Forwarded-For", r.Header.Get("X-Forwarded-For"))
			_, err = fmt.Fprintf(w, "%s|%s|%s|%s|%s|%s", name, r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), r.Header.Get("X-User-ID"), body)
			if err != nil {
				t.Errorf("write upstream response: %v", err)
			}
		}))
	}
	apiServer := upstream("api")
	defer apiServer.Close()
	notificationServer := upstream("notification")
	defer notificationServer.Close()
	apiURL, err := url.Parse(apiServer.URL)
	require.NoError(t, err)
	notificationURL, err := url.Parse(notificationServer.URL)
	require.NoError(t, err)
	gatewayServer := httptest.NewServer(NewHandler(apiURL, notificationURL))
	defer gatewayServer.Close()

	for _, tt := range []struct {
		method, path, body, upstream string
	}{
		{http.MethodPost, "/v1/auth/token?source=client", "token-request", "api"},
		{http.MethodGet, "/v1/feeds?limit=10", "", "api"},
		{http.MethodGet, "/v1/notification-channel", "", "notification"},
		{http.MethodPost, "/v1/notification-channel", `{"type":"email"}`, "notification"},
		{http.MethodPatch, "/v1/notification-channel/abc?audit=1", `{"enabled":false}`, "notification"},
		{http.MethodDelete, "/v1/notification-channel/abc", "", "notification"},
		{http.MethodGet, "/v1/notification-channel-other", "", "api"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			request, err := http.NewRequest(tt.method, gatewayServer.URL+tt.path, strings.NewReader(tt.body))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer test-token")
			request.Header.Set("X-User-ID", "forged-user")
			request.Header.Set("X-Forwarded-For", "203.0.113.9")
			response, err := gatewayServer.Client().Do(request)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			payload, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, tt.upstream, response.Header.Get("X-Upstream"))
			require.NotContains(t, response.Header.Get("X-Seen-Forwarded-For"), "203.0.113.9")
			require.Equal(t, fmt.Sprintf("%s|%s|%s|Bearer test-token||%s", tt.upstream, tt.method, tt.path, tt.body), string(payload))
		})
	}
	response, err := gatewayServer.Client().Get(gatewayServer.URL + "/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestGatewayReturnsBadGatewayWhenUpstreamUnavailable(t *testing.T) {
	unavailable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	upstreamURL, err := url.Parse(unavailable.URL)
	require.NoError(t, err)
	unavailable.Close()
	request := httptest.NewRequest(http.MethodGet, "/v1/notification-channel", nil)
	response := httptest.NewRecorder()
	NewHandler(upstreamURL, upstreamURL).ServeHTTP(response, request)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.JSONEq(t, `{"error":{"code":"bad_gateway","message":"upstream unavailable"}}`, response.Body.String())
}
