CREATE TABLE notification_channels (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('email', 'telegram')),
    destination TEXT NOT NULL CHECK (btrim(destination) <> '' AND octet_length(destination) <= 512),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_channels_user_id_channel_type_destination_key
        UNIQUE (user_id, channel_type, destination)
);

CREATE INDEX idx_notification_channels_enabled_user
    ON notification_channels (user_id, id)
    WHERE enabled = TRUE;
