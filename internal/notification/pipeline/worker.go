package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/textproto"
	"time"

	notificationemail "FeedFlow/internal/notification/email"
	"FeedFlow/internal/notification/retry"
	sender "FeedFlow/internal/notification/sender"
	notificationtelegram "FeedFlow/internal/notification/telegram"
)

type Worker struct {
	Store   Store
	Senders sender.Registry
	Policy  *retry.Policy
}

func (w Worker) Run(ctx context.Context) error {
	if w.Store == nil || w.Policy == nil || w.Senders == nil {
		return fmt.Errorf("delivery worker is not configured")
	}
	if _, err := w.Store.RecoverExpired(ctx, 100, 8); err != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "delivery recovery failed", "error", err)
	}
	poll := time.NewTicker(time.Second)
	recovery := time.NewTicker(time.Minute)
	defer poll.Stop()
	defer recovery.Stop()
	channelTypes := make([]string, 0, len(w.Senders))
	for channelType := range w.Senders {
		channelTypes = append(channelTypes, string(channelType))
	}
	for ctx.Err() == nil {
		select {
		case <-recovery.C:
			if _, err := w.Store.RecoverExpired(ctx, 100, 8); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "delivery recovery failed", "error", err)
			}
		default:
		}
		tasks, err := w.Store.Claim(ctx, 1, time.Minute, channelTypes)
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "claim deliveries failed", "error", err)
		}
		for _, task := range tasks {
			if ctx.Err() != nil {
				break
			}
			if err := w.Process(ctx, task); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "delivery processing failed", "delivery_id", task.ID, "error", err)
			}
		}
		if len(tasks) == 1 {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-poll.C:
		case <-recovery.C:
			if _, err := w.Store.RecoverExpired(ctx, 100, 8); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "delivery recovery failed", "error", err)
			}
		}
	}
	return nil
}

func (w Worker) Process(ctx context.Context, task Task) error {
	channelSender, err := w.Senders.Get(task.ChannelType)
	if err == nil {
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = channelSender.Send(sendCtx, task.Message)
		cancel()
	}
	if err == nil {
		return w.Store.MarkSent(ctx, task)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	delay, shouldRetry := w.Policy.NextDelay(ctx, task.Attempts, err)
	if !shouldRetry {
		delay = 0
	}
	return w.Store.MarkFailed(ctx, task, delay, safeErrorCode(err))
}

func safeErrorCode(err error) string {
	var telegramError *notificationtelegram.APIError
	if errors.As(err, &telegramError) {
		if telegramError.Code == 429 {
			return "telegram_rate_limited"
		}
		if telegramError.Code >= 500 {
			return "telegram_server_error"
		}
		return "telegram_rejected"
	}
	var transportError *notificationtelegram.TransportError
	if errors.As(err, &transportError) {
		return "telegram_transport_error"
	}
	var smtpError *notificationemail.SMTPError
	if errors.As(err, &smtpError) {
		var protocolError *textproto.Error
		if errors.As(err, &protocolError) {
			if protocolError.Code >= 400 && protocolError.Code < 500 {
				return "smtp_transient_error"
			}
			return "smtp_rejected"
		}
		return "smtp_transport_error"
	}
	if errors.Is(err, sender.ErrSenderNotRegistered) {
		return "sender_unconfigured"
	}
	return "provider_error"
}
