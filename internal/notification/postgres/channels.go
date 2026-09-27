package postgres

import (
	notification "FeedFlow/internal/notification/model"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	channelrepo "FeedFlow/internal/notification/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
)

const channelDestinationConstraint = "notification_channels_user_id_channel_type_destination_key"

func channelStorageError(operation string, err error) error {
	var pgErr *pgconn.PgError
	var connectErr *pgconn.ConnectError
	var networkErr net.Error
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == channelDestinationConstraint:
		return fmt.Errorf("%s: %w: %w", operation, channelrepo.ErrChannelConflict, err)
	case errors.Is(err, puddle.ErrClosedPool), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, io.EOF), errors.As(err, &connectErr), errors.As(err, &networkErr), pgconn.Timeout(err):
		return fmt.Errorf("%s: %w: %w", operation, channelrepo.ErrUnavailable, err)
	case pgErr != nil && (strings.HasPrefix(pgErr.Code, "08") || pgErr.Code == "53300" ||
		pgErr.Code == "57P01" || pgErr.Code == "57P02" || pgErr.Code == "57P03"):
		return fmt.Errorf("%s: %w: %w", operation, channelrepo.ErrUnavailable, err)
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}

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
		return notification.Channel{}, channelStorageError("create notification channel", err)
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
		return nil, channelStorageError("get user notification channels", err)
	}
	defer rows.Close()

	channels := make([]notification.Channel, 0)
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, channelStorageError("scan notification channel", err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, channelStorageError("iterate notification channels", err)
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
		return notification.Channel{}, fmt.Errorf("%w: %w", channelrepo.ErrChannelNotFound, err)
	}
	if err != nil {
		return notification.Channel{}, channelStorageError("set notification channel enabled", err)
	}
	return channel, nil
}

func (repo *Repository) DeleteChannel(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) error {
	query := `DELETE FROM notification_channels WHERE id = $1 AND user_id = $2`

	tag, err := repo.pool.Exec(ctx, query, channelID, userID)
	if err != nil {
		return channelStorageError("delete notification channel", err)
	}
	if tag.RowsAffected() != 1 {
		return channelrepo.ErrChannelNotFound
	}
	return nil
}
