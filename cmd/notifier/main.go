package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"FeedFlow/internal/notification/broker"
	"FeedFlow/internal/notification/contract"
	notificationemail "FeedFlow/internal/notification/email"
	notification "FeedFlow/internal/notification/model"
	"FeedFlow/internal/notification/pipeline"
	notificationpostgres "FeedFlow/internal/notification/postgres"
	"FeedFlow/internal/notification/retry"
	notificationsender "FeedFlow/internal/notification/sender"
	notificationtelegram "FeedFlow/internal/notification/telegram"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const (
	defaultSMTPPort    = 587
	defaultSMTPTimeout = 10 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("notification service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	_ = godotenv.Load()
	databaseURL := os.Getenv("NOTIFICATION_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("NOTIFICATION_DATABASE_URL is required")
	}
	dbPool, err := connectPostgres(appCtx, databaseURL)
	if err != nil {
		return err
	}
	defer dbPool.Close()

	senders, err := newSenderRegistry()
	if err != nil {
		return fmt.Errorf("create notification senders: %w", err)
	}
	publisher, err := broker.NewPublisher(os.Getenv("KAFKA_BROKERS"))
	if err != nil {
		return err
	}
	defer publisher.Close()
	consumerClient, err := broker.NewConsumer(os.Getenv("KAFKA_BROKERS"),
		"feedflow-notifications-v1", contract.RequestsTopicV1,
		contract.Retry1mTopicV1, contract.Retry10mTopicV1, contract.Retry1hTopicV1)
	if err != nil {
		return err
	}
	defer consumerClient.Close()
	repository := notificationpostgres.NewRepository(dbPool)
	consumer := pipeline.Consumer{Client: consumerClient, Store: repository, Publisher: publisher}
	worker := pipeline.Worker{Store: repository, Senders: senders, Policy: retry.DefaultPolicy()}
	ctx, cancel := context.WithCancel(appCtx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- consumer.Run(ctx) }()
	running := 1
	if len(senders) > 0 {
		go func() { results <- worker.Run(ctx) }()
		running++
	} else {
		slog.Warn("no delivery provider configured; accepted deliveries will remain pending")
	}
	slog.Info("notification service started")
	first := <-results
	cancel()
	if running == 2 {
		return errors.Join(first, <-results)
	}
	return first
}

func connectPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database pool config: %w", err)
	}

	connectCtx, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	defer cancelConnect()

	dbPool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := dbPool.Ping(connectCtx); err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return dbPool, nil
}

func newSenderRegistry() (notificationsender.Registry, error) {
	senders := make(map[notification.ChannelType]notificationsender.Sender, 2)

	telegramToken := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if telegramToken != "" {
		telegramSender, err := notificationtelegram.NewSender(telegramToken, nil)
		if err != nil {
			return nil, fmt.Errorf("create Telegram sender: %w", err)
		}
		senders[notification.ChannelTelegram] = telegramSender
	}

	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if smtpHost != "" {
		emailSender, err := newEmailSender(smtpHost)
		if err != nil {
			return nil, err
		}
		senders[notification.ChannelEmail] = emailSender
	}

	return notificationsender.NewRegistry(senders)
}

func newEmailSender(smtpHost string) (*notificationemail.Sender, error) {
	port, err := readIntEnv("SMTP_PORT", defaultSMTPPort)
	if err != nil {
		return nil, err
	}
	timeout, err := readDurationEnv("SMTP_TIMEOUT", defaultSMTPTimeout)
	if err != nil {
		return nil, err
	}

	emailSender, err := notificationemail.NewSender(notificationemail.Config{
		Host:        smtpHost,
		Port:        port,
		Username:    os.Getenv("SMTP_USERNAME"),
		Password:    os.Getenv("SMTP_PASSWORD"),
		FromAddress: os.Getenv("SMTP_FROM_ADDRESS"),
		FromName:    os.Getenv("SMTP_FROM_NAME"),
		TLSMode:     notificationemail.TLSMode(strings.TrimSpace(os.Getenv("SMTP_TLS_MODE"))),
		Timeout:     timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create email sender: %w", err)
	}
	return emailSender, nil
}

func readIntEnv(name string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}

func readDurationEnv(name string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}
