package postgres

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type channelScanner interface {
	Scan(dest ...any) error
}

func scanChannel(scanner channelScanner) (notification.Channel, error) {
	var channel notification.Channel
	var channelType string

	err := scanner.Scan(
		&channel.ID,
		&channel.UserID,
		&channelType,
		&channel.Destination,
		&channel.Enabled,
	)
	if err != nil {
		return notification.Channel{}, err
	}
	channel.Type = notification.ChannelType(channelType)
	return channel, nil
}

func (repo *Repository) CreateChannel(ctx context.Context, channel notification.Channel) (notification.Channel, error) {
	query := `INSERT INTO notification_channels (id, user_id, channel_type, destination, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, user_id, channel_type, destination, enabled
	`

	created, err := scanChannel(repo.pool.QueryRow(
		ctx,
		query,
		channel.ID,
		channel.UserID,
		string(channel.Type),
		channel.Destination,
	))
	if err != nil {
		return notification.Channel{}, fmt.Errorf("create notification channel: %w", err)
	}

	return created, nil
}

func (repo *Repository) GetChannels(ctx context.Context, userID uuid.UUID) ([]notification.Channel, error) {
	query := ` SELECT id, user_id, channel_type, destination, enabled FROM notification_channels
			WHERE user_id = $1
			ORDER BY created_at, id
	`

	rows, err := repo.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get user notification channels: %w", err)
	}
	defer rows.Close()

	channels := make([]notification.Channel, 0)
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan notification channel: %w", err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification channels: %w", err)
	}
	return channels, nil
}

func (repo *Repository) SetChannelEnabled(ctx context.Context, userID uuid.UUID, channelID uuid.UUID, enabled bool) (notification.Channel, error) {
	query := `UPDATE notification_channels SET enabled = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, channel_type,destination, enabled
	`

	channel, err := scanChannel(repo.pool.QueryRow(
		ctx,
		query,
		channelID,
		userID,
		enabled,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return notification.Channel{}, fmt.Errorf("notification channel not found: %w", err)
	}
	if err != nil {
		return notification.Channel{}, fmt.Errorf("set notification channel enabled: %w", err)
	}
	return channel, nil
}

func (repo *Repository) DeleteChannel(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) error {
	query := `DELETE FROM notification_channels WHERE id = $1 AND user_id = $2`

	tag, err := repo.pool.Exec(ctx, query, channelID, userID)
	if err != nil {
		return fmt.Errorf("delete notification channel: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("notification channel not found")
	}
	return nil
}
