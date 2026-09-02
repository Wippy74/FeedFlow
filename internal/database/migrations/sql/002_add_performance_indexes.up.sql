CREATE INDEX idx_feeds_fetch_order
    ON feeds (last_fetched_at ASC NULLS FIRST, id ASC);

CREATE INDEX idx_posts_feed_published_at
    ON posts (feed_id, published_at DESC);

CREATE INDEX idx_feed_follows_feed_id
    ON feed_follows (feed_id);
