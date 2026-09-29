package postgres

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"FeedFlow/internal/notification/contract"
	"FeedFlow/internal/notification/pipeline"
	"FeedFlow/internal/notification/relay"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type dispatchPublisherStub struct{ records []relay.Record }

func (p *dispatchPublisherStub) Publish(_ context.Context, record relay.Record) error {
	p.records = append(p.records, record)
	return nil
}

func TestPipelineIntegration(t *testing.T) {
	url := os.Getenv("FEEDFLOW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FEEDFLOW_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"pipeline_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, dropErr)
	})
	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	for _, name := range []string{"001_notification_channels", "002_notification_inbox",
		"003_notification_deliveries", "004_notification_dispatch_outbox"} {
		migration, readErr := os.ReadFile("../migrations/sql/" + name + ".up.sql")
		require.NoError(t, readErr)
		_, execErr := pool.Exec(ctx, string(migration), pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, execErr, name)
	}
	repo := NewRepository(pool)
	userID, channelID, eventID := uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO notification_channels
		(id, user_id, channel_type, destination) VALUES ($1, $2, 'email', 'test@example.com')`,
		channelID, userID)
	require.NoError(t, err)
	event := contract.Requested{EventID: eventID, NotificationID: eventID, UserID: userID,
		EventType: contract.NotificationRequested, SchemaVersion: 1, Template: contract.TemplateNewPost,
		Producer: "test", OccurredAt: time.Now().UTC(), Data: contract.RequestedData{
			PostID: uuid.New(), FeedID: uuid.New(), Title: "article", URL: "https://example.com/article",
			PublishedAt: time.Now().UTC()}}
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, repo.AcceptRequest(ctx, event, payload))
	require.NoError(t, repo.AcceptRequest(ctx, event, payload))
	changed := append([]byte(nil), payload...)
	changed = []byte(strings.Replace(string(changed), "article", "changed", 1))
	require.ErrorIs(t, repo.AcceptRequest(ctx, event, changed), pipeline.ErrIdentityConflict)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_deliveries WHERE event_id = $1`,
		eventID).Scan(&count))
	require.Equal(t, 1, count)

	tasks, err := repo.Claim(ctx, 1, time.Minute, []string{"email"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "test@example.com", tasks[0].Message.Recipient)
	require.NoError(t, repo.MarkFailed(ctx, tasks[0], time.Millisecond, "smtp_transient_error"))
	var retryPayload []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM notification_dispatch_outbox
		WHERE delivery_id = $1 AND kind = 'retry'`, tasks[0].ID).Scan(&retryPayload))
	var retryEvent contract.Retry
	require.NoError(t, json.Unmarshal(retryPayload, &retryEvent))
	require.NoError(t, repo.ResumeRetry(ctx, retryEvent))
	require.NoError(t, repo.ResumeRetry(ctx, retryEvent))
	tasks, err = repo.Claim(ctx, 1, time.Minute, []string{"email"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, 2, tasks[0].Attempts)
	require.NoError(t, repo.MarkFailed(ctx, tasks[0], 0, "smtp_rejected"))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_dispatch_outbox
		WHERE delivery_id = $1 AND kind = 'dlq'`, tasks[0].ID).Scan(&count))
	require.Equal(t, 1, count)
	require.ErrorIs(t, repo.MarkSent(ctx, tasks[0]), ErrStaleClaim)
	_, err = pool.Exec(ctx, `UPDATE notification_dispatch_outbox SET available_at = NOW()
		WHERE kind = 'retry'`)
	require.NoError(t, err)
	publisher := &dispatchPublisherStub{}
	dispatchRelay, err := relay.New(pool, publisher, relay.SourceDispatch)
	require.NoError(t, err)
	for range 2 {
		published, err := dispatchRelay.PublishOne(ctx)
		require.NoError(t, err)
		require.True(t, published)
	}
	require.Len(t, publisher.records, 2)
	topics := map[string]bool{}
	for _, record := range publisher.records {
		topics[record.Topic] = true
		require.Equal(t, userID.String(), record.Key)
	}
	require.True(t, topics[contract.Retry1mTopicV1])
	require.True(t, topics[contract.DLQTopicV1])

	secondID := uuid.New()
	event.EventID, event.NotificationID = secondID, secondID
	payload, err = json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, repo.AcceptRequest(ctx, event, payload))
	tasks, err = repo.Claim(ctx, 1, time.Minute, []string{"email"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	_, err = repo.SetChannelEnabled(ctx, userID, channelID, false)
	require.NoError(t, err)
	require.NoError(t, repo.MarkFailed(ctx, tasks[0], time.Second, "smtp_transient_error"))
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM notification_deliveries WHERE id = $1`,
		tasks[0].ID).Scan(&status))
	require.Equal(t, "cancelled", status)
}
