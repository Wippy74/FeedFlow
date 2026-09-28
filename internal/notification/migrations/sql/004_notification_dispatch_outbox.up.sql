CREATE TABLE notification_dispatch_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL,
    user_id UUID NOT NULL,
    attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
    kind TEXT NOT NULL CHECK (kind IN ('retry', 'dlq')),
    target_topic TEXT NOT NULL CHECK (target_topic IN (
        'feedflow.notification.retry.1m.v1',
        'feedflow.notification.retry.10m.v1',
        'feedflow.notification.retry.1h.v1',
        'feedflow.notification.dlq.v1'
    )),
    partition_key TEXT GENERATED ALWAYS AS (user_id::text) STORED,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_dispatch_outbox_delivery_user_fkey
        FOREIGN KEY (delivery_id, user_id) REFERENCES notification_deliveries(id, user_id),
    CONSTRAINT notification_dispatch_outbox_delivery_kind_attempt_key
        UNIQUE (delivery_id, kind, attempt_no),
    CONSTRAINT notification_dispatch_outbox_topic_kind_check CHECK (
        (kind = 'dlq' AND target_topic = 'feedflow.notification.dlq.v1')
        OR (kind = 'retry' AND target_topic <> 'feedflow.notification.dlq.v1')
    )
);

CREATE INDEX idx_notification_dispatch_outbox_pending
    ON notification_dispatch_outbox (available_at, created_at, id)
    WHERE published_at IS NULL;
