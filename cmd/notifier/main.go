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

	"FeedFlow/internal/closer"
	"FeedFlow/internal/config"
	notificationemail "FeedFlow/internal/notification/email"
	notification "FeedFlow/internal/notification/model"
	notificationpostgres "FeedFlow/internal/notification/postgres"
	"FeedFlow/internal/notification/retry"
	notificationsender "FeedFlow/internal/notification/sender"
	notificationtelegram "FeedFlow/internal/notification/telegram"
	notificationworker "FeedFlow/internal/notification/worker"

	"github.com/jackc/pgx/v5/pgxpool"
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

	cfg, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	dbPool, err := connectPostgres(appCtx, cfg)
	if err != nil {
		return err
	}

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

	senders, err := newSenderRegistry()
	if err != nil {
		return fmt.Errorf("create notification senders: %w", err)
	}

	service, err := notificationworker.New(
		notificationpostgres.NewRepository(dbPool),
		senders,
		retry.DefaultPolicy(),
		notificationworker.DefaultConfig(),
	)
	if err != nil {
		return fmt.Errorf("create notification worker: %w", err)
	}

	slog.Info("notification service started")
	if err := service.Run(appCtx); err != nil {
		return fmt.Errorf("run notification worker: %w", err)
	}
	slog.Info("notification service stopped")
	return nil
}

func connectPostgres(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		return nil, fmt.Errorf("parse database pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime

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

	if len(senders) == 0 {
		return nil, fmt.Errorf(
			"no notification channels configured: set TELEGRAM_BOT_TOKEN or SMTP_HOST",
		)
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
