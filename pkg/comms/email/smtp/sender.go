package smtp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	emailpkg "github.com/plaenen/eventstore/pkg/email"
)

// SMTPSender implements the EmailSender interface using SMTP
type SMTPSender struct {
	config *Config
}

// Config holds SMTP configuration
type Config struct {
	// SMTP server settings
	Host string
	Port int

	// Authentication
	Username string
	Password string

	// TLS settings
	UseTLS       bool // Use TLS from the start (SMTPS on port 465)
	UseSTARTTLS  bool // Use STARTTLS (on port 587 or 25)
	TLSConfig    *tls.Config
	SkipVerify   bool // Skip TLS certificate verification (not recommended for production)

	// Connection settings
	Timeout      time.Duration
	KeepAlive    time.Duration

	// Sender identity
	FromAddress string // Default "From" address if not specified in email
	FromName    string // Default "From" name

	// Limits
	MaxMessageSize int64 // Maximum message size in bytes (0 = no limit)
}

// Option is a functional option for configuring the SMTP sender
type Option func(*Config)

// WithHost sets the SMTP host
func WithHost(host string) Option {
	return func(c *Config) {
		c.Host = host
	}
}

// WithPort sets the SMTP port
func WithPort(port int) Option {
	return func(c *Config) {
		c.Port = port
	}
}

// WithAuth sets authentication credentials
func WithAuth(username, password string) Option {
	return func(c *Config) {
		c.Username = username
		c.Password = password
	}
}

// WithTLS enables direct TLS (SMTPS on port 465)
func WithTLS() Option {
	return func(c *Config) {
		c.UseTLS = true
		c.UseSTARTTLS = false
	}
}

// WithSTARTTLS enables STARTTLS (upgrade to TLS, typically on port 587)
func WithSTARTTLS() Option {
	return func(c *Config) {
		c.UseSTARTTLS = true
		c.UseTLS = false
	}
}

// WithTLSConfig sets a custom TLS configuration
func WithTLSConfig(tlsConfig *tls.Config) Option {
	return func(c *Config) {
		c.TLSConfig = tlsConfig
	}
}

// WithSkipVerify skips TLS certificate verification (not recommended for production!)
func WithSkipVerify() Option {
	return func(c *Config) {
		c.SkipVerify = true
	}
}

// WithTimeout sets the connection timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithKeepAlive sets the TCP keep-alive duration
func WithKeepAlive(keepAlive time.Duration) Option {
	return func(c *Config) {
		c.KeepAlive = keepAlive
	}
}

// WithFrom sets the default From address and optional name
func WithFrom(address string, name ...string) Option {
	return func(c *Config) {
		c.FromAddress = address
		if len(name) > 0 {
			c.FromName = name[0]
		}
	}
}

// WithMaxMessageSize sets the maximum message size in bytes
func WithMaxMessageSize(size int64) Option {
	return func(c *Config) {
		c.MaxMessageSize = size
	}
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Host:        "localhost",
		Port:        25,
		UseSTARTTLS: true,
		Timeout:     30 * time.Second,
		KeepAlive:   30 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

// NewSender creates a new SMTP email sender with functional options
//
// Example usage with options pattern (recommended):
//
//	sender := smtp.NewSender(
//	    smtp.WithHost("smtp.gmail.com"),
//	    smtp.WithPort(587),
//	    smtp.WithAuth("user@gmail.com", "app-password"),
//	    smtp.WithSTARTTLS(),
//	    smtp.WithFrom("user@gmail.com", "My Name"),
//	)
//
// Or with Config struct (backward compatible):
//
//	config := &smtp.Config{
//	    Host:        "smtp.gmail.com",
//	    Port:        587,
//	    Username:    "user@gmail.com",
//	    Password:    "app-password",
//	    UseSTARTTLS: true,
//	    FromAddress: "user@gmail.com",
//	}
//	sender := smtp.NewSender(config)
func NewSender(opts ...interface{}) *SMTPSender {
	// Start with defaults
	config := DefaultConfig()

	// Apply options
	for _, opt := range opts {
		switch v := opt.(type) {
		case *Config:
			// Backward compatibility: full Config struct
			config = v
		case Option:
			// Functional option
			v(config)
		}
	}

	// Apply defaults if not set
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.TLSConfig == nil {
		config.TLSConfig = &tls.Config{
			ServerName: config.Host,
			MinVersion: tls.VersionTLS12,
		}
	}
	if config.SkipVerify {
		config.TLSConfig.InsecureSkipVerify = true
	}
	if config.TLSConfig.ServerName == "" {
		config.TLSConfig.ServerName = config.Host
	}

	return &SMTPSender{
		config: config,
	}
}

// Send sends a single email via SMTP
func (s *SMTPSender) Send(ctx context.Context, email *emailpkg.Email) error {
	// Validate email
	if err := s.validateEmail(email); err != nil {
		return fmt.Errorf("email validation failed: %w", err)
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context error: %w", err)
	}

	// Build MIME message
	message, err := s.buildMessage(email)
	if err != nil {
		return fmt.Errorf("failed to build message: %w", err)
	}

	// Check size limit
	if s.config.MaxMessageSize > 0 && int64(len(message)) > s.config.MaxMessageSize {
		return fmt.Errorf("message size %d exceeds limit %d", len(message), s.config.MaxMessageSize)
	}

	// Connect and send
	if err := s.sendMessage(ctx, email, message); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// SendBatch sends multiple emails in a single SMTP session
func (s *SMTPSender) SendBatch(ctx context.Context, emails []*emailpkg.Email) error {
	if len(emails) == 0 {
		return nil
	}

	// For batch sending, we establish one connection and reuse it
	// This is more efficient than Send() for multiple messages
	client, err := s.connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	// Authenticate
	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Send each email
	for i, email := range emails {
		// Check context between sends
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context error after %d/%d emails: %w", i, len(emails), err)
		}

		if err := s.validateEmail(email); err != nil {
			return fmt.Errorf("email %d validation failed: %w", i, err)
		}

		message, err := s.buildMessage(email)
		if err != nil {
			return fmt.Errorf("email %d: failed to build message: %w", i, err)
		}

		// Set sender
		from := email.From
		if from == "" {
			from = s.config.FromAddress
		}
		if err := client.Mail(from); err != nil {
			return fmt.Errorf("email %d: failed to set sender: %w", i, err)
		}

		// Add recipients
		allRecipients := append([]string{}, email.To...)
		allRecipients = append(allRecipients, email.CC...)
		allRecipients = append(allRecipients, email.BCC...)

		for _, recipient := range allRecipients {
			if err := client.Rcpt(recipient); err != nil {
				return fmt.Errorf("email %d: failed to add recipient %s: %w", i, recipient, err)
			}
		}

		// Send data
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("email %d: failed to open data writer: %w", i, err)
		}

		if _, err := w.Write(message); err != nil {
			w.Close()
			return fmt.Errorf("email %d: failed to write message: %w", i, err)
		}

		if err := w.Close(); err != nil {
			return fmt.Errorf("email %d: failed to close data writer: %w", i, err)
		}

		// Reset for next message
		if err := client.Reset(); err != nil {
			return fmt.Errorf("email %d: failed to reset: %w", i, err)
		}
	}

	return nil
}

// validateEmail validates the email structure
func (s *SMTPSender) validateEmail(email *emailpkg.Email) error {
	if email == nil {
		return fmt.Errorf("email is nil")
	}

	// Validate sender
	from := email.From
	if from == "" {
		from = s.config.FromAddress
	}
	if from == "" {
		return fmt.Errorf("from address is required")
	}

	// Validate recipients
	if len(email.To) == 0 && len(email.CC) == 0 && len(email.BCC) == 0 {
		return fmt.Errorf("at least one recipient (To, CC, or BCC) is required")
	}

	// Validate content
	if email.TextBody == "" && email.HTMLBody == "" {
		return fmt.Errorf("email must have either text or HTML body")
	}

	return nil
}

// connect establishes an SMTP connection
func (s *SMTPSender) connect(ctx context.Context) (*smtp.Client, error) {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout:   s.config.Timeout,
		KeepAlive: s.config.KeepAlive,
	}

	var conn net.Conn
	var err error

	// Dial with context support
	if s.config.UseTLS {
		// Direct TLS (SMTPS on port 465)
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, s.config.TLSConfig)
	} else {
		// Plain connection (will potentially upgrade with STARTTLS)
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	// Create SMTP client
	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}

	// STARTTLS if not already using TLS
	if !s.config.UseTLS && s.config.UseSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(s.config.TLSConfig); err != nil {
				client.Close()
				return nil, fmt.Errorf("STARTTLS failed: %w", err)
			}
		}
	}

	return client, nil
}

// sendMessage sends a pre-built message
func (s *SMTPSender) sendMessage(ctx context.Context, email *emailpkg.Email, message []byte) error {
	client, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// Authenticate
	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Set sender
	from := email.From
	if from == "" {
		from = s.config.FromAddress
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Add recipients (To, CC, BCC)
	allRecipients := append([]string{}, email.To...)
	allRecipients = append(allRecipients, email.CC...)
	allRecipients = append(allRecipients, email.BCC...)

	for _, recipient := range allRecipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to add recipient %s: %w", recipient, err)
		}
	}

	// Send message data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data writer: %w", err)
	}
	defer w.Close()

	if _, err := w.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// Quit
	if err := client.Quit(); err != nil {
		// Quit errors are not critical
		return nil
	}

	return nil
}

// buildMessage constructs a MIME message from an Email
func (s *SMTPSender) buildMessage(email *emailpkg.Email) ([]byte, error) {
	var buf strings.Builder

	// Build headers
	if err := s.writeHeaders(&buf, email); err != nil {
		return nil, err
	}

	// Determine message structure
	hasAttachments := len(email.Attachments) > 0
	hasInlineAttachments := len(email.InlineAttachments) > 0
	hasMultipleContent := email.TextBody != "" && email.HTMLBody != ""

	// Simple message if no attachments and single content type
	if !hasAttachments && !hasInlineAttachments && !hasMultipleContent {
		// Simple message (single content type)
		if err := s.writeSimpleBody(&buf, email); err != nil {
			return nil, err
		}
	} else {
		// Complex multipart message
		if err := s.writeMultipartBody(&buf, email); err != nil {
			return nil, err
		}
	}

	return []byte(buf.String()), nil
}

// writeHeaders writes email headers
func (s *SMTPSender) writeHeaders(buf *strings.Builder, email *emailpkg.Email) error {
	// From
	from := email.From
	if from == "" {
		from = s.config.FromAddress
	}
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", s.config.FromName), from)
	}
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))

	// To
	if len(email.To) > 0 {
		buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(email.To, ", ")))
	}

	// CC
	if len(email.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(email.CC, ", ")))
	}

	// Note: BCC is not included in headers (by design)

	// Reply-To
	if email.ReplyTo != "" {
		buf.WriteString(fmt.Sprintf("Reply-To: %s\r\n", email.ReplyTo))
	}

	// Subject
	encodedSubject := mime.QEncoding.Encode("utf-8", email.Subject)
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", encodedSubject))

	// Date
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))

	// Message-ID
	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randomString(16), s.config.Host)
	buf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))

	// Priority
	if email.Priority > 0 && email.Priority != 3 {
		buf.WriteString(fmt.Sprintf("X-Priority: %d\r\n", email.Priority))
		if email.Priority == 1 {
			buf.WriteString("Importance: high\r\n")
		} else if email.Priority == 5 {
			buf.WriteString("Importance: low\r\n")
		}
	}

	// Custom headers
	for key, value := range email.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}

	// MIME version
	buf.WriteString("MIME-Version: 1.0\r\n")

	return nil
}

// writeSimpleBody writes a simple (non-multipart) message body
func (s *SMTPSender) writeSimpleBody(buf *strings.Builder, email *emailpkg.Email) error {
	if email.HTMLBody != "" {
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(quotedPrintableEncode(email.HTMLBody))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(quotedPrintableEncode(email.TextBody))
	}

	return nil
}

// writeMultipartBody writes a multipart message body
func (s *SMTPSender) writeMultipartBody(buf *strings.Builder, email *emailpkg.Email) error {
	hasAttachments := len(email.Attachments) > 0
	hasInlineAttachments := len(email.InlineAttachments) > 0

	// Create multipart writer
	boundary := randomBoundary()

	if hasAttachments {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n\r\n", boundary))
	} else if hasInlineAttachments {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=%s\r\n\r\n", boundary))
	} else {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", boundary))
	}

	// Write content parts
	if hasAttachments || hasInlineAttachments {
		// Create inner multipart for content
		innerBoundary := randomBoundary()
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))

		if hasInlineAttachments {
			buf.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=%s\r\n\r\n", innerBoundary))
		} else {
			buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", innerBoundary))
		}

		// Write text/html parts
		if err := s.writeContentParts(buf, email, innerBoundary); err != nil {
			return err
		}

		// Write inline attachments
		if hasInlineAttachments {
			for _, inline := range email.InlineAttachments {
				buf.WriteString(fmt.Sprintf("--%s\r\n", innerBoundary))
				buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", inline.ContentType))
				buf.WriteString(fmt.Sprintf("Content-Transfer-Encoding: base64\r\n"))
				buf.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", inline.ContentID))
				buf.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=%q\r\n\r\n", inline.Filename))
				buf.WriteString(base64Encode(inline.Data))
				buf.WriteString("\r\n")
			}
		}

		buf.WriteString(fmt.Sprintf("--%s--\r\n", innerBoundary))

		// Write attachments
		if hasAttachments {
			for _, att := range email.Attachments {
				data, err := s.loadAttachment(att)
				if err != nil {
					return fmt.Errorf("failed to load attachment %s: %w", att.Filename, err)
				}

				buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
				contentType := att.ContentType
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
				buf.WriteString("Content-Transfer-Encoding: base64\r\n")
				buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%q\r\n\r\n", att.Filename))
				buf.WriteString(base64Encode(data))
				buf.WriteString("\r\n")
			}
		}

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		// Simple multipart/alternative
		if err := s.writeContentParts(buf, email, boundary); err != nil {
			return err
		}
		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	}

	return nil
}

// writeContentParts writes text and HTML content parts
func (s *SMTPSender) writeContentParts(buf *strings.Builder, email *emailpkg.Email, boundary string) error {
	// Text part
	if email.TextBody != "" {
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		buf.WriteString(quotedPrintableEncode(email.TextBody))
		buf.WriteString("\r\n")
	}

	// HTML part
	if email.HTMLBody != "" {
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		buf.WriteString(quotedPrintableEncode(email.HTMLBody))
		buf.WriteString("\r\n")
	}

	return nil
}

// loadAttachment loads attachment data from various sources
func (s *SMTPSender) loadAttachment(att emailpkg.Attachment) ([]byte, error) {
	// Priority: Data > Reader > Path
	if len(att.Data) > 0 {
		return att.Data, nil
	}

	if att.Reader != nil {
		return io.ReadAll(att.Reader)
	}

	if att.Path != "" {
		data, err := os.ReadFile(att.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", att.Path, err)
		}

		// Auto-detect content type if not specified
		if att.ContentType == "" {
			att.ContentType = mime.TypeByExtension(filepath.Ext(att.Path))
		}

		return data, nil
	}

	return nil, fmt.Errorf("attachment has no data source")
}

// Utility functions

func randomBoundary() string {
	return fmt.Sprintf("----=_Part_%d_%s", time.Now().UnixNano(), randomString(16))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

func quotedPrintableEncode(s string) string {
	// Simple quoted-printable encoding
	var buf strings.Builder
	lineLen := 0

	for i := 0; i < len(s); i++ {
		c := s[i]

		// Check if character needs encoding
		needsEncoding := c < 33 || c > 126 || c == '='

		if needsEncoding {
			encoded := fmt.Sprintf("=%02X", c)
			if lineLen+len(encoded) > 75 {
				buf.WriteString("=\r\n")
				lineLen = 0
			}
			buf.WriteString(encoded)
			lineLen += len(encoded)
		} else {
			if lineLen >= 75 {
				buf.WriteString("=\r\n")
				lineLen = 0
			}
			buf.WriteByte(c)
			lineLen++
		}
	}

	return buf.String()
}

func base64Encode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)

	// Split into 76-character lines as per RFC 2045
	var buf strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		buf.WriteString(encoded[i:end])
		buf.WriteString("\r\n")
	}

	return buf.String()
}

// Additional helper methods

// TestConnection tests the SMTP connection without sending email
func (s *SMTPSender) TestConnection(ctx context.Context) error {
	client, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if s.config.Username != "" {
		auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client.Quit()
}

// Verify compiles check
var _ emailpkg.EmailSender = (*SMTPSender)(nil)
