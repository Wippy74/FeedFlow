package email

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"testing"
	"time"

	notification "FeedFlow/internal/notification/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingMailer struct {
	from       string
	recipients []string
	data       []byte
	err        error
}

func (m *recordingMailer) Send(
	_ context.Context,
	from string,
	recipients []string,
	data []byte,
) error {
	m.from = from
	m.recipients = append([]string(nil), recipients...)
	m.data = append([]byte(nil), data...)
	return m.err
}

func TestSenderSend(t *testing.T) {
	mailer := &recordingMailer{}
	sender, err := NewSenderWithMailer(
		"news@example.com",
		"FeedFlow",
		mailer,
	)
	require.NoError(t, err)
	sender.now = func() time.Time {
		return time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	}

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "reader@example.com",
		Title:     "Новая публикация",
		Body:      "Краткое описание",
		URL:       "https://example.com/post",
	})
	require.NoError(t, err)

	assert.Equal(t, "news@example.com", mailer.from)
	assert.Equal(t, []string{"reader@example.com"}, mailer.recipients)

	parsed, err := mail.ReadMessage(bytes.NewReader(mailer.data))
	require.NoError(t, err)
	assert.Equal(t, `"FeedFlow" <news@example.com>`, parsed.Header.Get("From"))
	assert.Equal(t, "<reader@example.com>", parsed.Header.Get("To"))

	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	require.NoError(t, err)
	assert.Equal(t, "Новая публикация", subject)

	body, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	require.NoError(t, err)
	assert.Equal(
		t,
		"Новая публикация\r\n\r\nКраткое описание\r\n\r\nhttps://example.com/post",
		string(body),
	)
}

func TestSenderRejectsInvalidAddressesAndHeaders(t *testing.T) {
	mailer := &recordingMailer{}

	_, err := NewSenderWithMailer("invalid", "FeedFlow", mailer)
	require.Error(t, err)

	sender, err := NewSenderWithMailer("news@example.com", "FeedFlow", mailer)
	require.NoError(t, err)

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "bad-address",
		Title:     "Title",
	})
	require.Error(t, err)

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "reader@example.com",
		Title:     "Title\r\nBcc: attacker@example.com",
	})
	require.Error(t, err)
}

func TestSenderReturnsMailerError(t *testing.T) {
	wantErr := errors.New("SMTP unavailable")
	mailer := &recordingMailer{err: wantErr}
	sender, err := NewSenderWithMailer("news@example.com", "", mailer)
	require.NoError(t, err)

	err = sender.Send(context.Background(), notification.Message{
		Recipient: "reader@example.com",
		Title:     "Title",
	})
	assert.ErrorIs(t, err, wantErr)
}

func TestSMTPErrorRetryable(t *testing.T) {
	temporary := &SMTPError{
		Operation: "send",
		Err:       &textproto.Error{Code: 451, Msg: "try again later"},
	}
	permanent := &SMTPError{
		Operation: "send",
		Err:       &textproto.Error{Code: 550, Msg: "mailbox unavailable"},
	}

	assert.True(t, temporary.Retryable())
	assert.False(t, permanent.Retryable())
}
