package main

import (
	cache2 "NewsAggregator/internal/cache"
	"NewsAggregator/internal/config"
	"NewsAggregator/internal/database/migrations"
	"NewsAggregator/internal/database/storage"
	"NewsAggregator/internal/handler"
	"NewsAggregator/internal/worker"
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.ReadConfig()
	if err != nil {
		log.Fatal(err)
		return
	}

	log.Println("Connecting to database")
	migrations.RunMigrations(cfg.DBUrl)

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	dbPool, err := pgxpool.New(connectCtx, cfg.DBUrl)
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
	defer dbPool.Close()
	log.Println("Connected to database")

	repo := storage.NewRepository(dbPool)
	cache := cache2.NewRedisCache(cfg.RedisAddr)
	go func() {
		err := worker.Start(repo, 1*time.Minute, 3)
		if err != nil {
			log.Fatal(err)
		}
	}()

	apiHandler := handler.NewHandler(repo, cache)
	router := apiHandler.InitRouter()
	port := ":8080"
	server := &http.Server{
		Addr:         port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Listening on port %s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Error during server running: %v", err)
	}
}
