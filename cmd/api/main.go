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

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5"
)

func main() {
	cfg, err := config.ReadConfig()
	if err != nil {
		log.Fatal(err)
		return
	}

	log.Println("Connecting to database")
	migrations.RunMigrations(cfg.DBUrl)

	conn, err := pgx.Connect(context.Background(), cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer conn.Close(context.Background())
	log.Println("Connected to database")

	repo := storage.NewRepository(conn)
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
