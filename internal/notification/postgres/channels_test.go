package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	channelrepo "FeedFlow/internal/notification/repository"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/puddle/v2"
	"github.com/stretchr/testify/assert"
)

func TestChannelStorageErrorClassification(t *testing.T) {
	for _, tt := range []struct {
		name        string
		cause, want error
	}{
		{"duplicate destination", &pgconn.PgError{Code: "23505", ConstraintName: channelDestinationConstraint}, channelrepo.ErrChannelConflict},
		{"duplicate generated ID is a bug", &pgconn.PgError{Code: "23505", ConstraintName: "notification_channels_pkey"}, nil},
		{"foreign key failure", &pgconn.PgError{Code: "23503"}, nil},
		{"SQL bug", &pgconn.PgError{Code: "42P01"}, nil},
		{"closed pool", puddle.ErrClosedPool, channelrepo.ErrUnavailable},
		{"deadline", context.DeadlineExceeded, channelrepo.ErrUnavailable},
		{"network", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, channelrepo.ErrUnavailable},
		{"EOF", io.EOF, channelrepo.ErrUnavailable},
		{"connect error", &pgconn.ConnectError{}, channelrepo.ErrUnavailable},
		{"connection SQL state", &pgconn.PgError{Code: "08006"}, channelrepo.ErrUnavailable},
		{"too many connections", &pgconn.PgError{Code: "53300"}, channelrepo.ErrUnavailable},
		{"shutdown", &pgconn.PgError{Code: "57P01"}, channelrepo.ErrUnavailable},
		{"unexpected", errors.New("unknown error"), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := channelStorageError("test", fmt.Errorf("wrapped: %w", tt.cause))
			assert.ErrorIs(t, err, tt.cause)
			for _, sentinel := range []error{channelrepo.ErrChannelConflict, channelrepo.ErrUnavailable, channelrepo.ErrChannelNotFound} {
				assert.Equal(t, errors.Is(sentinel, tt.want), errors.Is(err, sentinel))
			}
		})
	}
}
