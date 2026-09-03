package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShutdownCancelsAndWaitsForBackgroundTasks(t *testing.T) {
	h := NewHandler(&MockStorage{}, &MockCache{})
	started := make(chan struct{})
	finished := make(chan struct{})

	h.runInBackground(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	<-started

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, h.Shutdown(shutdownCtx))

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("background task did not finish during shutdown")
	}
}
