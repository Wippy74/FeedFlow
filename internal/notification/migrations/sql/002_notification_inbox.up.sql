CREATE TABLE notification_inbox (
    event_id UUID PRIMARY KEY,
    notification_id UUID NOT NULL UNIQUE,
    user_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type = 'notification.requested'),
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    template TEXT NOT NULL CHECK (template = 'new_post'),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_inbox_identity_check CHECK (event_id = notification_id),
    CONSTRAINT notification_inbox_event_user_key UNIQUE (event_id, user_id)
);
