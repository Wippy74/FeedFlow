CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL,
    user_id UUID NOT NULL,
    channel_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'retry_wait', 'sent', 'failed', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_token UUID,
    lease_expires_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    provider_receipt TEXT,
    last_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_deliveries_event_user_fkey
        FOREIGN KEY (event_id, user_id) REFERENCES notification_inbox(event_id, user_id),
    CONSTRAINT notification_deliveries_id_user_key UNIQUE (id, user_id),
    CONSTRAINT notification_deliveries_event_channel_key UNIQUE (event_id, channel_id),
    CONSTRAINT notification_deliveries_lease_check CHECK (
        (status = 'processing') = (claim_token IS NOT NULL AND lease_expires_at IS NOT NULL)
    ),
    CONSTRAINT notification_deliveries_sent_check CHECK (
        (status = 'sent') = (sent_at IS NOT NULL)
    )
);

CREATE INDEX idx_notification_deliveries_pending
    ON notification_deliveries (available_at, created_at, id)
    WHERE status = 'pending';

CREATE INDEX idx_notification_deliveries_expired_lease
    ON notification_deliveries (lease_expires_at, id)
    WHERE status = 'processing';

CREATE INDEX idx_notification_deliveries_user
    ON notification_deliveries (user_id, created_at DESC);
