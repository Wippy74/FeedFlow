package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	notification "FeedFlow/internal/notification/model"
)

const defaultTimeout = 10 * time.Second

type TLSMode string

const (
	TLSModeSTARTTLS TLSMode = "starttls"
	TLSModeImplicit TLSMode = "implicit"
	TLSModeNone     TLSMode = "none"
)

type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	TLSMode     TLSMode
	Timeout     time.Duration
}

type Mailer interface {
	Send(ctx context.Context, from string, recipients []string, data []byte) error
}

type Sender struct {
	from   mail.Address
	mailer Mailer
	now    func() time.Time
}

type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	tlsMode  TLSMode
	timeout  time.Duration
}

type SMTPError struct {
	Operation string
	Err       error
}

func (e *SMTPError) Error() string {
	return fmt.Sprintf("smtp %s: %v", e.Operation, e.Err)
}

func (e *SMTPError) Unwrap() error {
	return e.Err
}

func (e *SMTPError) Retryable() bool {
	var protocolErr *textproto.Error
	if errors.As(e.Err, &protocolErr) {
		return protocolErr.Code >= 400 && protocolErr.Code < 500
	}
	var networkErr net.Error
	return errors.As(e.Err, &networkErr) ||
		errors.Is(e.Err, io.EOF) ||
		errors.Is(e.Err, io.ErrUnexpectedEOF)
}

func NewSender(config Config) (*Sender, error) {
	mailer, err := NewSMTPMailer(config)
	if err != nil {
		return nil, err
	}
	return NewSenderWithMailer(config.FromAddress, config.FromName, mailer)
}

func NewSenderWithMailer(fromAddress string, fromName string, mailer Mailer) (*Sender, error) {
	if mailer == nil {
		return nil, fmt.Errorf("email mailer must not be nil")
	}
	if containsNewline(fromName) {
		return nil, fmt.Errorf("email sender name contains a newline")
	}

	from, err := parseSingleAddress(fromAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid email sender address: %w", err)
	}
	from.Name = strings.TrimSpace(fromName)

	return &Sender{
		from:   from,
		mailer: mailer,
		now:    time.Now,
	}, nil
}

func NewSMTPMailer(config Config) (*SMTPMailer, error) {
	host := strings.TrimSpace(config.Host)
	if host == "" {
		return nil, fmt.Errorf("SMTP host must not be empty")
	}
	if strings.ContainsAny(host, "\r\n\t ") {
		return nil, fmt.Errorf("SMTP host contains invalid characters")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return nil, fmt.Errorf("SMTP port must be between 1 and 65535")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("SMTP username and password must be configured together")
	}

	tlsMode := config.TLSMode
	if tlsMode == "" {
		tlsMode = TLSModeSTARTTLS
	}
	switch tlsMode {
	case TLSModeSTARTTLS, TLSModeImplicit, TLSModeNone:
	default:
		return nil, fmt.Errorf("unsupported SMTP TLS mode %q", tlsMode)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("SMTP timeout must not be negative")
	}

	return &SMTPMailer{
		host:     host,
		port:     config.Port,
		username: config.Username,
		password: config.Password,
		tlsMode:  tlsMode,
		timeout:  timeout,
	}, nil
}

func (s *Sender) Send(ctx context.Context, message notification.Message) error {
	if s == nil {
		return fmt.Errorf("email sender is nil")
	}
	if s.mailer == nil {
		return fmt.Errorf("email mailer is nil")
	}

	recipient, err := parseSingleAddress(message.Recipient)
	if err != nil {
		return fmt.Errorf("invalid email recipient: %w", err)
	}

	subject := strings.TrimSpace(message.Title)
	if subject == "" {
		subject = "New publication"
	}
	if containsNewline(subject) {
		return fmt.Errorf("email subject contains a newline")
	}

	body := buildBody(message)
	if body == "" {
		return fmt.Errorf("email message must not be empty")
	}

	data, err := buildMIMEMessage(s.from, recipient, subject, body, s.now().UTC())
	if err != nil {
		return fmt.Errorf("build email message: %w", err)
	}

	if err := s.mailer.Send(
		ctx,
		s.from.Address,
		[]string{recipient.Address},
		data,
	); err != nil {
		return err
	}

	return nil
}

func (m *SMTPMailer) Send(ctx context.Context, from string, recipients []string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients must not be empty")
	}

	address := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	dialer := &net.Dialer{Timeout: m.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return &SMTPError{Operation: "connect", Err: err}
	}

	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stopCancellation()
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(m.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return &SMTPError{Operation: "set deadline", Err: err}
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: m.host,
	}
	if m.tlsMode == TLSModeImplicit {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return &SMTPError{Operation: "TLS handshake", Err: err}
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return &SMTPError{Operation: "create client", Err: err}
	}
	defer func() { _ = client.Close() }()

	if m.tlsMode == TLSModeSTARTTLS {
		available, _ := client.Extension("STARTTLS")
		if !available {
			return &SMTPError{
				Operation: "STARTTLS",
				Err:       fmt.Errorf("server does not advertise STARTTLS"),
			}
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return &SMTPError{Operation: "STARTTLS", Err: err}
		}
	}

	if m.username != "" {
		auth := smtp.PlainAuth("", m.username, m.password, m.host)
		if err := client.Auth(auth); err != nil {
			return &SMTPError{Operation: "authenticate", Err: err}
		}
	}

	if err := client.Mail(from); err != nil {
		return &SMTPError{Operation: "set sender", Err: err}
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return &SMTPError{Operation: "set recipient", Err: err}
		}
	}

	writer, err := client.Data()
	if err != nil {
		return &SMTPError{Operation: "start message body", Err: err}
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return &SMTPError{Operation: "write message body", Err: err}
	}
	if err := writer.Close(); err != nil {
		return &SMTPError{Operation: "finish message body", Err: err}
	}

	_ = client.Quit()
	return nil
}

func parseSingleAddress(value string) (mail.Address, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return mail.Address{}, fmt.Errorf("address must not be empty")
	}
	if containsNewline(value) {
		return mail.Address{}, fmt.Errorf("address contains a newline")
	}

	address, err := mail.ParseAddress(value)
	if err != nil {
		return mail.Address{}, err
	}
	return *address, nil
}

func buildBody(message notification.Message) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{message.Title, message.Body, message.URL} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildMIMEMessage(from mail.Address, to mail.Address, subject string, body string, createdAt time.Time) ([]byte, error) {
	var message bytes.Buffer

	headers := []string{
		"From: " + from.String(),
		"To: " + to.String(),
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject),
		"Date: " + createdAt.Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
	}
	for _, header := range headers {
		message.WriteString(header)
		message.WriteString("\r\n")
	}
	message.WriteString("\r\n")

	writer := quotedprintable.NewWriter(&message)
	if _, err := writer.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return message.Bytes(), nil
}

func containsNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}
