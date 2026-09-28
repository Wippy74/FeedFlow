package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"FeedFlow/internal/config"
	"FeedFlow/internal/gateway"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("gateway stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.ReadGatewayConfig()
	if err != nil {
		return fmt.Errorf("read gateway config: %w", err)
	}
	apiURL, _ := url.Parse(cfg.APIURL)
	notificationURL, _ := url.Parse(cfg.NotificationURL)
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on gateway address: %w", err)
	}
	server := &http.Server{
		Handler:           gateway.NewHandler(apiURL, notificationURL),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() {
		if cfg.TLSCertPath != "" {
			serverDone <- server.ServeTLS(listener, cfg.TLSCertPath, cfg.TLSKeyPath)
			return
		}
		serverDone <- server.Serve(listener)
	}()
	slog.Info("gateway started", "address", listener.Addr().String(), "tls", cfg.TLSCertPath != "")
	select {
	case <-ctx.Done():
		slog.Info("gateway shutdown signal received")
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("gateway HTTP server stopped: %w", err)
		}
		return nil
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown gateway HTTP server: %w", err)
	}
	return nil
}
