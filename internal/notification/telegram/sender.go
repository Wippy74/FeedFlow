package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	notification "FeedFlow/internal/notification/model"
)

const (
	defaultAPIBaseURL = "https://api.telegram.org"
	maxMessageRunes   = 4096
	maxResponseBytes  = 1 << 20
	defaultTimeout    = 10 * time.Second
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Sender struct {
	token      string
	client     HTTPClient
	apiBaseURL string
}

//kubernetik

type APIError struct {
	HTTPStatus  int
	Code        int
	Description string
	RetryAfter  time.Duration
}

func (e *APIError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("telegram API error %d: %s", e.Code, e.Description)
	}
	return fmt.Sprintf("telegram API error %d", e.Code)
}

func (e *APIError) Retryable() bool {
	return e.HTTPStatus == http.StatusTooManyRequests ||
		e.HTTPStatus >= http.StatusInternalServerError ||
		e.Code == http.StatusTooManyRequests ||
		e.Code >= http.StatusInternalServerError
}

type TransportError struct {
	err error
}

func (e *TransportError) Error() string {
	return "telegram transport request failed"
}

func (e *TransportError) Unwrap() error {
	return e.err
}

type sendMessageRequest struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func NewSender(token string, client HTTPClient) (*Sender, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("telegram bot token must not be empty")
	}
	if strings.ContainsAny(token, "/\r\n\t ") {
		return nil, fmt.Errorf("telegram bot token contains invalid characters")
	}

	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	return &Sender{
		token:      token,
		client:     client,
		apiBaseURL: defaultAPIBaseURL,
	}, nil
}

func (s *Sender) Send(ctx context.Context, message notification.Message) error {
	if s == nil {
		return fmt.Errorf("telegram sender is nil")
	}
	if s.client == nil {
		return fmt.Errorf("telegram HTTP client is nil")
	}

	recipient := strings.TrimSpace(message.Recipient)
	if recipient == "" {
		return fmt.Errorf("telegram recipient must not be empty")
	}

	text, err := formatMessage(message)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(sendMessageRequest{
		ChatID: recipient,
		Text:   text,
	})
	if err != nil {
		return fmt.Errorf("encode telegram sendMessage request: %w", err)
	}

	endpoint := strings.TrimRight(s.apiBaseURL, "/") +
		"/bot" + s.token + "/sendMessage"
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create telegram sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return &TransportError{err: err}
	}
	if resp == nil {
		return fmt.Errorf("telegram returned a nil HTTP response")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read telegram response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("telegram response exceeds %d bytes", maxResponseBytes)
	}

	var result sendMessageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf(
			"decode telegram response with HTTP status %d: %w",
			resp.StatusCode,
			err,
		)
	}

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices ||
		!result.OK {
		code := result.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}

		return &APIError{
			HTTPStatus:  resp.StatusCode,
			Code:        code,
			Description: result.Description,
			RetryAfter:  time.Duration(result.Parameters.RetryAfter) * time.Second,
		}
	}

	return nil
}

func formatMessage(message notification.Message) (string, error) {
	parts := make([]string, 0, 3)
	for _, part := range []string{message.Title, message.Body, message.URL} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("telegram message must not be empty")
	}

	text := strings.Join(parts, "\n\n")
	runes := []rune(text)
	if len(runes) > maxMessageRunes {
		text = string(runes[:maxMessageRunes])
	}

	return text, nil
}
