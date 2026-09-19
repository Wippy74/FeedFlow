package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"FeedFlow/internal/model"
	notification "FeedFlow/internal/notification/model"
	notificationpostgres "FeedFlow/internal/notification/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavePostIntegrationCreatesOneOutboxEvent(t *testing.T) {
	databaseURL := os.Getenv("FEEDFLOW_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("FEEDFLOW_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	feedID := uuid.New()
	userID := uuid.New()
	followID := uuid.New()
	channelID := uuid.New()
	postID := uuid.New()
	duplicatePostID := uuid.New()
	postURL := "https://example.com/" + postID.String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, err = pool.Exec(
		ctx,
		`INSERT INTO users (id, created_at, updated_at, name, api_key) VALUES ($1, $2, $2, $3, $4)`,
		userID,
		now,
		"integration user",
		"integration-key-"+userID.String(),
	)
	require.NoError(t, err)

	_, err = pool.Exec(
		ctx,
		`INSERT INTO feeds (id, created_at, updated_at, name, url) VALUES ($1, $2, $2, $3, $4)`,
		feedID,
		now,
		"integration feed",
		"https://example.com/feed/"+feedID.String(),
	)
	require.NoError(t, err)

	_, err = pool.Exec(
		ctx,
		`INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id) VALUES ($1, $2, $2, $3, $4)`,
		followID,
		now,
		userID,
		feedID,
	)
	require.NoError(t, err)

	_, err = pool.Exec(
		ctx,
		`INSERT INTO notification_channels (id, user_id, channel_type, destination, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $5)`,
		channelID,
		userID,
		notification.ChannelEmail,
		"integration@example.com",
		now,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM notification_deliveries WHERE post_id = $1`, postID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_events WHERE aggregate_id = $1`, postID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM posts WHERE id IN ($1, $2)`, postID, duplicatePostID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notification_channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM feed_follows WHERE id = $1`, followID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM feeds WHERE id = $1`, feedID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	repository := NewRepository(pool)
	post := model.Post{
		ID:          postID,
		Title:       "integration post",
		Description: sql.NullString{String: "integration description", Valid: true},
		PublishedAt: now.Add(-time.Minute),
		Url:         postURL,
		FeedID:      feedID,
	}

	inserted, err := repository.SavePost(ctx, post)
	require.NoError(t, err)
	assert.True(t, inserted)

	var eventType notification.EventType
	var payloadBytes []byte
	err = pool.QueryRow(
		ctx,
		`SELECT event_type, payload FROM outbox_events WHERE aggregate_id = $1`,
		postID,
	).Scan(&eventType, &payloadBytes)
	require.NoError(t, err)
	assert.Equal(t, notification.EventPostCreated, eventType)

	var payload postCreatedPayload
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	assert.Equal(t, postID, payload.Data.PostID)
	assert.Equal(t, feedID, payload.Data.FeedID)
	assert.Equal(t, postCreatedSchemaVersion, payload.SchemaVersion)

	expanded, err := notificationpostgres.NewRepository(pool).ExpandPostCreatedEvents(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), expanded)

	var deliveryCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM notification_deliveries WHERE post_id = $1 AND user_id = $2 AND channel_id = $3`,
		postID,
		userID,
		channelID,
	).Scan(&deliveryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, deliveryCount)

	var processed bool
	err = pool.QueryRow(
		ctx,
		`SELECT processed_at IS NOT NULL FROM outbox_events WHERE aggregate_id = $1`,
		postID,
	).Scan(&processed)
	require.NoError(t, err)
	assert.True(t, processed)

	post.ID = duplicatePostID
	inserted, err = repository.SavePost(ctx, post)
	require.NoError(t, err)
	assert.False(t, inserted)

	var outboxCount int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1`,
		postID,
	).Scan(&outboxCount)
	require.NoError(t, err)
	assert.Equal(t, 1, outboxCount)
}
