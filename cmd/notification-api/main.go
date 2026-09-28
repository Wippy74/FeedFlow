package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"FeedFlow/internal/auth"
	"FeedFlow/internal/config"
	notificationhttp "FeedFlow/internal/notification/httpapi"
	notificationpostgres "FeedFlow/internal/notification/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("notification API stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.ReadNotificationAPIConfig()
	if err != nil {
		return fmt.Errorf("read notification API config: %w", err)
	}
	publicKey, err := auth.LoadPublicKey(cfg.PublicKeyPath)
	if err != nil {
		return err
	}
	verifier, err := auth.NewVerifier(publicKey, cfg.KeyID, cfg.Issuer, auth.AudienceNotifications)
	if err != nil {
		return err
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid NOTIFICATION_DATABASE_URL")
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		cancelConnect()
		return fmt.Errorf("create notification database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		cancelConnect()
		return fmt.Errorf("ping notification database: %w", err)
	}
	cancelConnect()

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on notification HTTP address: %w", err)
	}
	server := &http.Server{
		Handler:           notificationhttp.NewHandler(notificationpostgres.NewRepository(pool)).Router(verifier),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	slog.Info("notification HTTP API started", "address", listener.Addr().String())
	select {
	case <-ctx.Done():
		slog.Info("notification HTTP API shutdown signal received")
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("notification HTTP server stopped: %w", err)
		}
		return nil
	}
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown notification HTTP server: %w", err)
	}
	return nil
}
