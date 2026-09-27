package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type database interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db          database
	now         func() time.Time
	idGenerator func() uuid.UUID
}

func NewRepository(db database) *Repository {
	return &Repository{
		db:          db,
		now:         time.Now,
		idGenerator: uuid.New,
	}
}
