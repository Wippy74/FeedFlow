package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayStartsAndStops(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	t.Setenv("GATEWAY_ADDR", address)
	t.Setenv("GATEWAY_API_URL", "http://127.0.0.1:18082")
	t.Setenv("GATEWAY_NOTIFICATION_URL", "http://127.0.0.1:18081")
	t.Setenv("GATEWAY_TLS_CERT_PATH", "")
	t.Setenv("GATEWAY_TLS_KEY_PATH", "")
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	require.Eventually(t, func() bool {
		response, requestErr := client.Get("http://" + address + "/healthz")
		if requestErr != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode == http.StatusNoContent
	}, 5*time.Second, 20*time.Millisecond)
	stop()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("gateway did not stop after context cancellation")
	}
}
