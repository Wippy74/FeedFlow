package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	notification "FeedFlow/internal/notification/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type httpClientFunc func(*http.Request) (*http.Response, error)

func (fn httpClientFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSenderSend(t *testing.T) {
	client := httpClientFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/bottest-token/sendMessage", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var request sendMessageRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "123456", request.ChatID)
		assert.Equal(t, "Title\n\nDescription\n\nhttps://example.com/post", request.Text)

		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"ok":true,"result":{"message_id":1}}`,
			)),
			Header: make(http.Header),
		}, nil
	})

	sender, err := NewSender("test-token", client)
	require.NoError(t, err)

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "123456",
		Title:     "Title",
		Body:      "Description",
		URL:       "https://example.com/post",
	})
	require.NoError(t, err)
}

func TestSenderReturnsTelegramAPIError(t *testing.T) {
	client := httpClientFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body: io.NopCloser(strings.NewReader(`{
				"ok":false,
				"error_code":429,
				"description":"Too Many Requests",
				"parameters":{"retry_after":7}
			}`)),
			Header: make(http.Header),
		}, nil
	})

	sender, err := NewSender("test-token", client)
	require.NoError(t, err)

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "123456",
		Title:     "Title",
	})

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.Code)
	assert.Equal(t, 7*time.Second, apiErr.RetryAfter)
	assert.True(t, apiErr.Retryable())
}

func TestSenderValidation(t *testing.T) {
	_, err := NewSender("", nil)
	require.Error(t, err)

	sender, err := NewSender("test-token", nil)
	require.NoError(t, err)

	err = sender.Send(context.Background(), notification.Message{Title: "Title"})
	require.Error(t, err)

	err = sender.Send(context.Background(), notification.Message{Recipient: "123456"})
	require.Error(t, err)
}

func TestFormatMessageLimitsTelegramLength(t *testing.T) {
	text, err := formatMessage(notification.Message{
		Body: strings.Repeat("я", maxMessageRunes+100),
	})
	require.NoError(t, err)
	assert.Len(t, []rune(text), maxMessageRunes)
}

func TestTransportErrorUnwrapsCauseWithoutPrintingIt(t *testing.T) {
	cause := errors.New("request URL contains secret token")
	err := &TransportError{err: cause}

	assert.Equal(t, "telegram transport request failed", err.Error())
	assert.ErrorIs(t, err, cause)
}
