ALTER TABLE outbox_events
    ADD COLUMN target_topic TEXT,
    ADD COLUMN partition_key TEXT,
    ADD COLUMN schema_version INTEGER,
    ADD COLUMN published_at TIMESTAMPTZ,
    ADD CONSTRAINT outbox_notification_route_check CHECK (
        event_type <> 'notification.requested'
        OR (
            target_topic IS NOT NULL AND target_topic <> ''
            AND partition_key IS NOT NULL AND partition_key <> ''
            AND schema_version IS NOT NULL AND schema_version > 0
        )
    );

DROP INDEX idx_outbox_pending;

CREATE INDEX idx_outbox_pending
    ON outbox_events (available_at, created_at)
    WHERE processed_at IS NULL AND event_type = 'post.created';

CREATE UNIQUE INDEX idx_outbox_notification_intent
    ON outbox_events (aggregate_id, partition_key)
    WHERE event_type = 'notification.requested';

CREATE INDEX idx_outbox_notification_unpublished
    ON outbox_events (created_at, id)
    WHERE target_topic IS NOT NULL AND published_at IS NULL;
