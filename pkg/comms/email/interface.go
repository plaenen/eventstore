package email

import (
	"context"
	"io"
	"time"
)

// EmailSender is the main interface for sending emails
type EmailSender interface {
	Send(ctx context.Context, email *Email) error
	SendBatch(ctx context.Context, emails []*Email) error
}

// Email represents a complete email message
type Email struct {
	// Basic fields
	From    string
	To      []string
	Subject string

	// CC and BCC
	CC  []string
	BCC []string

	// Reply-To
	ReplyTo string

	// Content (provide one or both)
	TextBody string // Plain text version
	HTMLBody string // HTML version

	// Attachments
	Attachments []Attachment

	// Inline attachments (for embedding images in HTML)
	InlineAttachments []InlineAttachment

	// Headers
	Headers map[string]string

	// Priority (1=highest, 3=normal, 5=lowest)
	Priority int

	// Metadata
	Tags     []string          // For tracking/categorization
	Metadata map[string]string // Custom metadata

	// Scheduling (if provider supports it)
	SendAt *time.Time

	// Tracking options
	TrackOpens  bool
	TrackClicks bool
}

// Attachment represents a file attachment
type Attachment struct {
	Filename    string
	ContentType string

	// Provide one of these:
	Data   []byte    // Raw bytes
	Reader io.Reader // Stream
	Path   string    // File path
}

// InlineAttachment represents an embedded image or file
type InlineAttachment struct {
	ContentID   string // Used in HTML as <img src="cid:logo">
	Filename    string
	ContentType string
	Data        []byte
}

// SendOptions provides additional sending configuration
type SendOptions struct {
	DryRun     bool
	Timeout    time.Duration
	RetryCount int
	RetryDelay time.Duration
}

// SendResult contains information about the sent email
type SendResult struct {
	MessageID string
	Success   bool
	Error     error
	Provider  string
	Timestamp time.Time
}

// Validator validates email addresses and content
type EmailValidator interface {
	ValidateEmail(email string) error
	ValidateContent(email *Email) error
}

// TemplateRenderer renders email templates
type TemplateRenderer interface {
	RenderHTML(templateName string, data interface{}) (string, error)
	RenderText(templateName string, data interface{}) (string, error)
}

// Builder provides a fluent interface for constructing emails
type EmailBuilder struct {
	email *Email
}

func NewEmail() *EmailBuilder {
	return &EmailBuilder{
		email: &Email{
			Headers:  make(map[string]string),
			Metadata: make(map[string]string),
			Priority: 3, // Normal priority by default
		},
	}
}

func (b *EmailBuilder) From(from string) *EmailBuilder {
	b.email.From = from
	return b
}

func (b *EmailBuilder) To(to ...string) *EmailBuilder {
	b.email.To = append(b.email.To, to...)
	return b
}

func (b *EmailBuilder) CC(cc ...string) *EmailBuilder {
	b.email.CC = append(b.email.CC, cc...)
	return b
}

func (b *EmailBuilder) BCC(bcc ...string) *EmailBuilder {
	b.email.BCC = append(b.email.BCC, bcc...)
	return b
}

func (b *EmailBuilder) ReplyTo(replyTo string) *EmailBuilder {
	b.email.ReplyTo = replyTo
	return b
}

func (b *EmailBuilder) Subject(subject string) *EmailBuilder {
	b.email.Subject = subject
	return b
}

func (b *EmailBuilder) TextBody(text string) *EmailBuilder {
	b.email.TextBody = text
	return b
}

func (b *EmailBuilder) HTMLBody(html string) *EmailBuilder {
	b.email.HTMLBody = html
	return b
}

func (b *EmailBuilder) Attach(filename, contentType string, data []byte) *EmailBuilder {
	b.email.Attachments = append(b.email.Attachments, Attachment{
		Filename:    filename,
		ContentType: contentType,
		Data:        data,
	})
	return b
}

func (b *EmailBuilder) AttachFile(filepath string) *EmailBuilder {
	b.email.Attachments = append(b.email.Attachments, Attachment{
		Path: filepath,
	})
	return b
}

func (b *EmailBuilder) InlineImage(contentID, filename, contentType string, data []byte) *EmailBuilder {
	b.email.InlineAttachments = append(b.email.InlineAttachments, InlineAttachment{
		ContentID:   contentID,
		Filename:    filename,
		ContentType: contentType,
		Data:        data,
	})
	return b
}

func (b *EmailBuilder) Header(key, value string) *EmailBuilder {
	b.email.Headers[key] = value
	return b
}

func (b *EmailBuilder) Priority(priority int) *EmailBuilder {
	b.email.Priority = priority
	return b
}

func (b *EmailBuilder) Tag(tags ...string) *EmailBuilder {
	b.email.Tags = append(b.email.Tags, tags...)
	return b
}

func (b *EmailBuilder) Meta(key, value string) *EmailBuilder {
	b.email.Metadata[key] = value
	return b
}

func (b *EmailBuilder) TrackOpens() *EmailBuilder {
	b.email.TrackOpens = true
	return b
}

func (b *EmailBuilder) TrackClicks() *EmailBuilder {
	b.email.TrackClicks = true
	return b
}

func (b *EmailBuilder) SendAt(sendAt time.Time) *EmailBuilder {
	b.email.SendAt = &sendAt
	return b
}

func (b *EmailBuilder) Build() *Email {
	return b.email
}
