package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cache "NewsAggregator/internal/cache"
	"NewsAggregator/internal/closer"
	"NewsAggregator/internal/config"
	"NewsAggregator/internal/database/migrations"
	"NewsAggregator/internal/database/storage"
	"NewsAggregator/internal/handler"
	"NewsAggregator/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	cfg, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	slog.Info("connecting to database")
	if err := migrations.RunMigrations(cfg.DBUrl); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		return fmt.Errorf("parse database pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime

	connectCtx, cancelConnect := context.WithTimeout(appCtx, 10*time.Second)
	dbPool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		cancelConnect()
		return fmt.Errorf("create database pool: %w", err)
	}
	if err := dbPool.Ping(connectCtx); err != nil {
		cancelConnect()
		dbPool.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	cancelConnect()

	resourceCloser := closer.New()
	if err := resourceCloser.Add("PostgreSQL pool", func() error {
		dbPool.Close()
		return nil
	}); err != nil {
		dbPool.Close()
		return fmt.Errorf("register PostgreSQL pool closer: %w", err)
	}
	defer func() {
		runErr = errors.Join(runErr, resourceCloser.Close())
	}()
	slog.Info("connected to database",
		"max_connections", cfg.DBMaxConns,
		"min_connections", cfg.DBMinConns,
	)

	dbRepo := storage.NewRepository(dbPool)
	cacheRepo := cache.NewRedisCache(cfg.RedisAddr)
	if err := resourceCloser.Add("Redis client", cacheRepo.Close); err != nil {
		_ = cacheRepo.Close()
		return fmt.Errorf("register Redis client closer: %w", err)
	}

	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.Start(appCtx, dbRepo, time.Minute, 3)
	}()

	apiHandler := handler.NewHandler(dbRepo, cacheRepo)
	server := &http.Server{
		Addr:         ":8080",
		Handler:      apiHandler.InitRouter(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.ListenAndServe()
	}()

	slog.Info("HTTP server started", "address", server.Addr)
	workerFinished := false
	select {
	case <-appCtx.Done():
		slog.Info("shutdown signal received")
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("HTTP server stopped unexpectedly: %w", err)
		}
		stopSignals()
	case err := <-workerDone:
		workerFinished = true
		if err != nil {
			runErr = fmt.Errorf("RSS worker stopped unexpectedly: %w", err)
		}
		stopSignals()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("graceful HTTP shutdown: %w", err))
		if closeErr := server.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("forced HTTP shutdown: %w", closeErr))
		}
	}
	if err := apiHandler.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("stop background cache operations: %w", err))
	}

	if !workerFinished {
		select {
		case err := <-workerDone:
			if err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("stop RSS worker: %w", err))
			}
		case <-shutdownCtx.Done():
			runErr = errors.Join(runErr, fmt.Errorf("wait for RSS worker: %w", shutdownCtx.Err()))
		}
	}

	slog.Info("application stopped")
	return runErr
}
