package relay

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type publisherStub struct {
	records []Record
	err     error
}

func (p *publisherStub) Publish(_ context.Context, record Record) error {
	if p.err != nil {
		return p.err
	}
	p.records = append(p.records, record)
	return nil
}

func TestRelayIntegrationAcknowledgesBeforeMarkingPublished(t *testing.T) {
	url := os.Getenv("FEEDFLOW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FEEDFLOW_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"relay_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
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
	for _, name := range []string{"006_outbox_events", "008_notification_kafka_outbox"} {
		migration, readErr := os.ReadFile("../../database/migrations/sql/" + name + ".up.sql")
		require.NoError(t, readErr)
		_, execErr := pool.Exec(ctx, string(migration), pgx.QueryExecModeSimpleProtocol)
		require.NoError(t, execErr, name)
	}
	id, postID, userID := uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO outbox_events
		(id, event_type, aggregate_id, payload, created_at, available_at, target_topic, partition_key, schema_version)
		VALUES ($1, 'notification.requested', $2, $3::jsonb, NOW(), NOW(), $4, $5, 1)`,
		id, postID, `{"event_id":"`+id.String()+`"}`, "feedflow.notification.requests.v1", userID.String())
	require.NoError(t, err)
	publisher := &publisherStub{err: errors.New("broker unavailable")}
	service, err := New(pool, publisher, SourceRequests)
	require.NoError(t, err)
	published, err := service.PublishOne(ctx)
	require.Error(t, err)
	require.False(t, published)
	var attempts int
	var marked bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT attempts, published_at IS NOT NULL
		FROM outbox_events WHERE id = $1`, id).Scan(&attempts, &marked))
	require.Equal(t, 1, attempts)
	require.False(t, marked)
	_, err = pool.Exec(ctx, `UPDATE outbox_events SET available_at = NOW() WHERE id = $1`, id)
	require.NoError(t, err)
	publisher.err = nil
	published, err = service.PublishOne(ctx)
	require.NoError(t, err)
	require.True(t, published)
	require.Len(t, publisher.records, 1)
	require.Equal(t, id.String(), publisher.records[0].Headers["event_id"])
	require.Equal(t, userID.String(), publisher.records[0].Key)
	require.NoError(t, pool.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE id = $1`,
		id).Scan(&marked))
	require.True(t, marked)
}
