package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"FeedFlow/internal/model"
	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type databaseStub struct {
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (stub *databaseStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("unexpected Exec call")
}

func (stub *databaseStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query call")
}

func (stub *databaseStub) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return stub.queryRowFn(ctx, query, args...)
}

type rowStub struct {
	inserted bool
	err      error
}

func (row rowStub) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	*(destinations[0].(*bool)) = row.inserted
	return nil
}

func TestSavePostWritesPostAndOutboxAtomically(t *testing.T) {
	now := time.Date(2026, time.September, 19, 12, 30, 0, 0, time.UTC)
	eventID := uuid.MustParse("df571fcc-b1d7-4a0f-892e-6b74de3a7218")
	postID := uuid.MustParse("06e711e1-46e2-4dd3-9a1d-13d910f38310")
	feedID := uuid.MustParse("03ba41b1-4fcf-4532-a865-2fbd0542b133")
	description := "description"

	var capturedQuery string
	var capturedArgs []any
	db := &databaseStub{queryRowFn: func(_ context.Context, query string, args ...any) pgx.Row {
		capturedQuery = query
		capturedArgs = args
		return rowStub{inserted: true}
	}}
	repository := NewRepository(db)
	repository.now = func() time.Time { return now }
	repository.idGenerator = func() uuid.UUID { return eventID }

	inserted, err := repository.SavePost(context.Background(), model.Post{
		ID:          postID,
		Title:       "title",
		Description: sql.NullString{String: description, Valid: true},
		PublishedAt: now.Add(-time.Hour),
		Url:         "https://example.com/post",
		FeedID:      feedID,
	})

	require.NoError(t, err)
	assert.True(t, inserted)
	assert.Contains(t, capturedQuery, "WITH inserted_post AS")
	assert.Contains(t, capturedQuery, "INSERT INTO posts")
	assert.Contains(t, capturedQuery, "INSERT INTO outbox_events")
	assert.Contains(t, capturedQuery, "FROM inserted_post")
	require.Len(t, capturedArgs, 10)
	assert.Equal(t, eventID, capturedArgs[7])
	assert.Equal(t, notification.EventPostCreated, capturedArgs[8])

	var payload postCreatedPayload
	require.NoError(t, json.Unmarshal(capturedArgs[9].([]byte), &payload))
	assert.Equal(t, eventID, payload.EventID)
	assert.Equal(t, notification.EventPostCreated, payload.EventType)
	assert.Equal(t, postCreatedSchemaVersion, payload.SchemaVersion)
	assert.Equal(t, now, payload.OccurredAt)
	assert.Equal(t, postCreatedProducer, payload.Producer)
	assert.Equal(t, postID, payload.Data.PostID)
	assert.Equal(t, feedID, payload.Data.FeedID)
	require.NotNil(t, payload.Data.Description)
	assert.Equal(t, description, *payload.Data.Description)
}

func TestSavePostDoesNotCreateOutboxEventForDuplicatePost(t *testing.T) {
	db := &databaseStub{queryRowFn: func(_ context.Context, query string, _ ...any) pgx.Row {
		assert.True(t, strings.Contains(query, "FROM inserted_post"))
		return rowStub{inserted: false}
	}}
	repository := NewRepository(db)

	inserted, err := repository.SavePost(context.Background(), model.Post{})

	require.NoError(t, err)
	assert.False(t, inserted)
}

func TestSavePostReturnsDatabaseError(t *testing.T) {
	databaseError := errors.New("database unavailable")
	db := &databaseStub{queryRowFn: func(context.Context, string, ...any) pgx.Row {
		return rowStub{err: databaseError}
	}}
	repository := NewRepository(db)

	inserted, err := repository.SavePost(context.Background(), model.Post{})

	assert.False(t, inserted)
	require.ErrorIs(t, err, databaseError)
}
