package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"FeedFlow/internal/model"
	"FeedFlow/internal/notification/contract"
	notification "FeedFlow/internal/notification/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavePostIntegrationCreatesNotificationIntents(t *testing.T) {
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
	secondUserID := uuid.New()
	followID := uuid.New()
	secondFollowID := uuid.New()
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
		`INSERT INTO users (id, created_at, updated_at, name, api_key) VALUES ($1, $2, $2, $3, $4)`,
		secondUserID,
		now,
		"second integration user",
		"integration-key-"+secondUserID.String(),
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
		secondFollowID,
		now,
		secondUserID,
		feedID,
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM feed_follows WHERE id = $1`, secondFollowID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM feeds WHERE id = $1`, feedID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, secondUserID)
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

	rows, err := pool.Query(
		ctx,
		`SELECT id, target_topic, partition_key, schema_version, payload
		 FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`,
		postID,
		contract.NotificationRequested,
	)
	require.NoError(t, err)
	defer rows.Close()
	intentUsers := make(map[uuid.UUID]bool)
	for rows.Next() {
		var intentID uuid.UUID
		var topic, partitionKey string
		var version int
		var value []byte
		require.NoError(t, rows.Scan(&intentID, &topic, &partitionKey, &version, &value))

		var requested contract.Requested
		require.NoError(t, json.Unmarshal(value, &requested))
		assert.Equal(t, intentID, requested.EventID)
		assert.Equal(t, intentID, requested.NotificationID)
		assert.Equal(t, contract.NotificationRequested, requested.EventType)
		assert.Equal(t, "feedflow-monolith", requested.Producer)
		assert.False(t, requested.OccurredAt.IsZero())
		assert.Equal(t, contract.RequestsTopicV1, topic)
		assert.Equal(t, requested.UserID.String(), partitionKey)
		assert.Equal(t, contract.RequestedSchemaV1, version)
		assert.Equal(t, version, requested.SchemaVersion)
		assert.Equal(t, contract.TemplateNewPost, requested.Template)
		assert.Equal(t, postID, requested.Data.PostID)
		assert.Equal(t, feedID, requested.Data.FeedID)
		assert.Equal(t, post.Title, requested.Data.Title)
		assert.Equal(t, post.Description.String, requested.Data.Body)
		assert.Equal(t, post.Url, requested.Data.URL)
		assert.True(t, post.PublishedAt.Equal(requested.Data.PublishedAt))
		intentUsers[requested.UserID] = true
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[uuid.UUID]bool{userID: true, secondUserID: true}, intentUsers)
	rows.Close()

	var unpublishedIntents int
	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2 AND published_at IS NULL`,
		postID,
		contract.NotificationRequested,
	).Scan(&unpublishedIntents)
	require.NoError(t, err)
	assert.Equal(t, 2, unpublishedIntents)

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
	assert.Equal(t, 2, outboxCount)
}
