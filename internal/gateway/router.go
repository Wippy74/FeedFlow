package gateway

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	api "FeedFlow/internal/httpapi"
)

func NewHandler(apiURL, notificationURL *url.URL) http.Handler {
	apiProxy := newProxy("api", apiURL)
	notificationProxy := newProxy("notification-api", notificationURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v1/notification-channel" || strings.HasPrefix(r.URL.Path, "/v1/notification-channel/") {
			notificationProxy.ServeHTTP(w, r)
			return
		}
		apiProxy.ServeHTTP(w, r)
	})
}

func newProxy(name string, target *url.URL) *httputil.ReverseProxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
			for header := range request.Out.Header {
				if strings.HasPrefix(strings.ToLower(header), "x-user-") {
					delete(request.Out.Header, header)
				}
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			slog.ErrorContext(r.Context(), "gateway upstream unavailable", "upstream", name)
			api.WriteError(w, http.StatusBadGateway, "bad_gateway", "upstream unavailable")
		},
	}
}
