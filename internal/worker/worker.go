package worker

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"FeedFlow/internal/model"

	"github.com/google/uuid"
)

const (
	feedRequestTimeout = 10 * time.Second
	maxFeedSize        = 5 << 20
)

type FeedStorage interface {
	GetNextFeedsToFetch(ctx context.Context, limit int) ([]model.Feed, error)
	MarkFeedFetched(ctx context.Context, id uuid.UUID) error
	SavePost(ctx context.Context, post model.Post) (bool, error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func Start(ctx context.Context, db FeedStorage, fetchInterval time.Duration, concurrency int) error {
	if fetchInterval <= 0 {
		return fmt.Errorf("fetch interval must be greater than zero")
	}
	if concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}

	client := &http.Client{Timeout: feedRequestTimeout}
	ticker := time.NewTicker(fetchInterval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := fetchBatch(ctx, db, client, concurrency); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.ErrorContext(ctx, "failed to get feeds for update", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func fetchBatch(ctx context.Context, db FeedStorage, client HTTPClient, concurrency int) error {
	queryCtx, cancel := context.WithTimeout(ctx, feedRequestTimeout)
	feeds, err := db.GetNextFeedsToFetch(queryCtx, concurrency)
	cancel()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, feed := range feeds {
		wg.Add(1)
		go func(feed model.Feed) {
			defer wg.Done()
			inserted, err := scrapeFeed(ctx, db, client, feed)
			if err != nil {
				if ctx.Err() == nil {
					slog.ErrorContext(ctx, "failed to update feed",
						"feed_id", feed.ID.String(),
						"feed_name", feed.Name,
						"error", err,
					)
				}
				return
			}
			slog.InfoContext(ctx, "feed updated",
				"feed_id", feed.ID.String(),
				"feed_name", feed.Name,
				"new_posts", inserted,
			)
		}(feed)
	}
	wg.Wait()
	return nil
}

func scrapeFeed(ctx context.Context, db FeedStorage, client HTTPClient, feed model.Feed) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.Url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSize+1))
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > maxFeedSize {
		return 0, fmt.Errorf("feed exceeds maximum size of %d bytes", maxFeedSize)
	}

	var rss RSSFeed
	if err := xml.Unmarshal(body, &rss); err != nil {
		return 0, fmt.Errorf("parse XML: %w", err)
	}

	insertedPosts := 0
	for _, item := range rss.Channel.Items {
		if err := ctx.Err(); err != nil {
			return insertedPosts, err
		}

		title := strings.TrimSpace(item.Title)
		link := strings.TrimSpace(item.Link)
		if title == "" || link == "" {
			slog.WarnContext(ctx, "skipping invalid feed item",
				"feed_id", feed.ID.String(),
				"feed_name", feed.Name,
				"reason", "title or link is empty",
			)
			continue
		}

		publishedAt, err := parsePublishedAt(item.PubDate)
		if err != nil {
			slog.WarnContext(ctx, "skipping feed item",
				"feed_id", feed.ID.String(),
				"feed_name", feed.Name,
				"post_title", title,
				"error", err,
			)
			continue
		}

		description := strings.TrimSpace(item.Description)
		inserted, err := db.SavePost(ctx, model.Post{
			ID:          uuid.New(),
			Title:       title,
			Description: sql.NullString{String: description, Valid: description != ""},
			PublishedAt: publishedAt,
			Url:         link,
			FeedID:      feed.ID,
		})
		if err != nil {
			return insertedPosts, fmt.Errorf("save post %q: %w", title, err)
		}
		if inserted {
			insertedPosts++
		}
	}

	if err := db.MarkFeedFetched(ctx, feed.ID); err != nil {
		return insertedPosts, fmt.Errorf("mark feed as fetched: %w", err)
	}
	return insertedPosts, nil
}

func parsePublishedAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("publication date is empty")
	}

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.RFC3339,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported publication date %q", value)
}
