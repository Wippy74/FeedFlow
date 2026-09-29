DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM outbox_events WHERE event_type = 'notification.requested') THEN
        RAISE EXCEPTION 'delete or migrate notification.requested outbox events before rolling back migration 008';
    END IF;
END
$$;

DROP INDEX idx_outbox_notification_unpublished;
DROP INDEX idx_outbox_notification_intent;
DROP INDEX idx_outbox_pending;

CREATE INDEX idx_outbox_pending
    ON outbox_events (available_at, created_at)
    WHERE processed_at IS NULL;

ALTER TABLE outbox_events
    DROP CONSTRAINT outbox_notification_route_check,
    DROP COLUMN published_at,
    DROP COLUMN schema_version,
    DROP COLUMN partition_key,
    DROP COLUMN target_topic;
