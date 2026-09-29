package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"FeedFlow/internal/notification/broker"
	"FeedFlow/internal/notification/relay"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("outbox relay stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	_ = godotenv.Load()
	source := relay.Source(os.Getenv("RELAY_SOURCE"))
	databaseURL := os.Getenv("RELAY_DATABASE_URL")
	if databaseURL == "" && source == relay.SourceDispatch {
		databaseURL = os.Getenv("NOTIFICATION_DATABASE_URL")
	}
	if databaseURL == "" && source == relay.SourceRequests {
		databaseURL = monolithDatabaseURL()
	}
	if databaseURL == "" {
		return fmt.Errorf("RELAY_DATABASE_URL is required")
	}
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("open relay database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("ping relay database: %w", err)
	}
	publisher, err := broker.NewPublisher(os.Getenv("KAFKA_BROKERS"))
	if err != nil {
		return err
	}
	defer publisher.Close()
	service, err := relay.New(pool, publisher, source)
	if err != nil {
		return err
	}
	slog.Info("outbox relay started", "source", source)
	return service.Run(ctx)
}

func monolithDatabaseURL() string {
	user, password := os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")
	host, port, name := os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME")
	if user == "" || host == "" || port == "" || name == "" {
		return ""
	}
	address := url.URL{Scheme: "postgres", User: url.UserPassword(user, password),
		Host: net.JoinHostPort(host, port), Path: "/" + name}
	query := address.Query()
	query.Set("sslmode", "disable")
	address.RawQuery = query.Encode()
	return address.String()
}
