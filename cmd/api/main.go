package main

import (
	"context"
	"errors"
	"log"
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
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	cfg, err := config.ReadConfig()
	if err != nil {
		log.Fatal(err)
		return
	}

	log.Println("Connecting to database")
	migrations.RunMigrations(cfg.DBUrl)

	poolConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		log.Fatalf("failed to parse database pool config: %v", err)
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime

	connectCtx, cancelConnect := context.WithTimeout(appCtx, 10*time.Second)
	dbPool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		cancelConnect()
		log.Fatalf("failed to create database pool: %v", err)
	}

	if err := dbPool.Ping(connectCtx); err != nil {
		cancelConnect()
		dbPool.Close()
		log.Fatalf("failed to connect to database: %v", err)
	}
	cancelConnect()

	resourceCloser := closer.New()
	if err := resourceCloser.Add("PostgreSQL pool", func() error {
		dbPool.Close()
		return nil
	}); err != nil {
		dbPool.Close()
		log.Printf("failed to register PostgreSQL pool closer: %v", err)
		return
	}
	defer func() {
		if err := resourceCloser.Close(); err != nil {
			log.Printf("failed to close application resources: %v", err)
		}
	}()
	log.Println("Connected to database")

	dbRepo := storage.NewRepository(dbPool)
	cacheRepo := cache.NewRedisCache(cfg.RedisAddr)
	if err := resourceCloser.Add("Redis client", cacheRepo.Close); err != nil {
		_ = cacheRepo.Close()
		log.Printf("failed to register Redis client closer: %v", err)
		return
	}

	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.Start(appCtx, dbRepo, 1*time.Minute, 3)
	}()

	apiHandler := handler.NewHandler(dbRepo, cacheRepo)
	router := apiHandler.InitRouter()
	port := ":8080"
	server := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.ListenAndServe()
	}()

	log.Printf("Listening on port %s", port)
	workerFinished := false
	select {
	case <-appCtx.Done():
		log.Println("Shutdown signal received")
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server stopped unexpectedly: %v", err)
		}
		stopSignals()
	case err := <-workerDone:
		workerFinished = true
		if err != nil {
			log.Printf("RSS worker stopped unexpectedly: %v", err)
		}
		stopSignals()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful HTTP shutdown failed: %v", err)
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("Forced HTTP shutdown failed: %v", closeErr)
		}
	}
	if err := apiHandler.Shutdown(shutdownCtx); err != nil {
		log.Printf("Timed out waiting for background cache operations: %v", err)
	}

	if !workerFinished {
		select {
		case err := <-workerDone:
			if err != nil {
				log.Printf("RSS worker stopped with error: %v", err)
			}
		case <-shutdownCtx.Done():
			log.Println("Timed out waiting for RSS worker to stop")
		}
	}

	log.Println("Application stopped")
}
