package worker

import (
	"FeedFlow/internal/model"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workerStorageMock struct {
	getFeedsFn func(context.Context, int) ([]model.Feed, error)
	markFeedFn func(context.Context, uuid.UUID) error
	savePostFn func(context.Context, model.Post) (bool, error)
}

type httpClientFunc func(*http.Request) (*http.Response, error)

func (fn httpClientFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func responseClient(status int, body string) HTTPClient {
	return httpClientFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
}

func (m *workerStorageMock) GetNextFeedsToFetch(ctx context.Context, limit int) ([]model.Feed, error) {
	if m.getFeedsFn != nil {
		return m.getFeedsFn(ctx, limit)
	}
	return nil, nil
}

func (m *workerStorageMock) MarkFeedFetched(ctx context.Context, id uuid.UUID) error {
	if m.markFeedFn != nil {
		return m.markFeedFn(ctx, id)
	}
	return nil
}

func (m *workerStorageMock) SavePost(ctx context.Context, post model.Post) (bool, error) {
	if m.savePostFn != nil {
		return m.savePostFn(ctx, post)
	}
	return true, nil
}

func TestStartStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped := make(chan error, 1)
	go func() {
		stopped <- Start(ctx, nil, time.Hour, 1)
	}()

	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestStartFetchesImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan struct{})
	storage := &workerStorageMock{
		getFeedsFn: func(context.Context, int) ([]model.Feed, error) {
			close(called)
			cancel()
			return nil, nil
		},
	}
	stopped := make(chan error, 1)
	go func() {
		stopped <- Start(ctx, storage, time.Hour, 1)
	}()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("worker did not fetch feeds immediately")
	}
	require.NoError(t, <-stopped)
}

func TestScrapeFeedMarksSuccessAndCountsOnlyInsertedPosts(t *testing.T) {
	feedID := uuid.New()
	rss := `<?xml version="1.0"?><rss><channel>
		<item><title>New post</title><link>https://example.com/new</link><description> New </description><pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate></item>
		<item><title>Duplicate</title><link>https://example.com/duplicate</link><pubDate>2006-01-02T15:04:05Z</pubDate></item>
		<item><title>Bad date</title><link>https://example.com/bad-date</link><pubDate>not-a-date</pubDate></item>
		<item><title>Missing link</title><pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate></item>
	</channel></rss>`
	var saved []model.Post
	marked := false
	storage := &workerStorageMock{
		savePostFn: func(_ context.Context, post model.Post) (bool, error) {
			saved = append(saved, post)
			return post.Url != "https://example.com/duplicate", nil
		},
		markFeedFn: func(_ context.Context, id uuid.UUID) error {
			assert.Equal(t, feedID, id)
			marked = true
			return nil
		},
	}

	inserted, err := scrapeFeed(context.Background(), storage, responseClient(http.StatusOK, rss), model.Feed{
		ID: feedID, Name: "Test feed", Url: "https://example.com/rss",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, inserted)
	assert.Len(t, saved, 2)
	assert.Equal(t, "New", saved[0].Description.String)
	assert.True(t, marked)
}

func TestScrapeFeedDoesNotMarkFailedFeed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "HTTP error",
			status: http.StatusBadGateway,
		},
		{
			name:   "oversized response",
			status: http.StatusOK,
			body:   strings.Repeat("x", maxFeedSize+1),
		},
		{
			name:   "unsupported XML format",
			status: http.StatusOK,
			body:   `<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom</title></feed>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marked := false
			storage := &workerStorageMock{
				markFeedFn: func(context.Context, uuid.UUID) error {
					marked = true
					return nil
				},
			}

			_, err := scrapeFeed(context.Background(), storage, responseClient(tt.status, tt.body), model.Feed{ID: uuid.New(), Url: "https://example.com/rss"})

			require.Error(t, err)
			assert.False(t, marked)
		})
	}
}

func TestScrapeFeedDoesNotMarkWhenPostSaveFails(t *testing.T) {
	rss := `<rss><channel><item><title>Post</title><link>https://example.com/post</link><pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate></item></channel></rss>`
	marked := false
	storage := &workerStorageMock{
		savePostFn: func(context.Context, model.Post) (bool, error) {
			return false, errors.New("database unavailable")
		},
		markFeedFn: func(context.Context, uuid.UUID) error {
			marked = true
			return nil
		},
	}

	_, err := scrapeFeed(context.Background(), storage, responseClient(http.StatusOK, rss), model.Feed{ID: uuid.New(), Url: "https://example.com/rss"})

	require.Error(t, err)
	assert.False(t, marked)
}

func TestParsePublishedAt(t *testing.T) {
	tests := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"02 Jan 06 15:04 -0700",
		"2006-01-02T15:04:05Z",
	}
	for _, value := range tests {
		parsed, err := parsePublishedAt(value)
		require.NoError(t, err)
		assert.Equal(t, time.UTC, parsed.Location())
	}

	_, err := parsePublishedAt("not-a-date")
	require.Error(t, err)
}
