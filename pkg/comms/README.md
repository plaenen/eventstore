# Communications Package

The `comms` package provides communication abstractions for sending notifications, alerts, and messages through various channels. Currently, it includes a comprehensive email subsystem with SMTP support.

## Package Structure

```
pkg/comms/
├── email/
│   ├── interface.go      # Core interfaces and email builder
│   └── smtp/
│       ├── sender.go     # SMTP implementation
│       └── *_test.go     # Unit and integration tests
└── README.md
```

## Email Subsystem

### Core Interface

The `email.EmailSender` interface defines the contract for sending emails:

```go
type EmailSender interface {
    Send(ctx context.Context, email *Email) error
    SendBatch(ctx context.Context, emails []*Email) error
}
```

### Email Structure

The `Email` struct supports all common email features:

| Field | Type | Description |
|-------|------|-------------|
| `From` | `string` | Sender email address |
| `To` | `[]string` | Primary recipients |
| `CC` | `[]string` | Carbon copy recipients |
| `BCC` | `[]string` | Blind carbon copy recipients |
| `ReplyTo` | `string` | Reply-to address |
| `Subject` | `string` | Email subject |
| `TextBody` | `string` | Plain text content |
| `HTMLBody` | `string` | HTML content |
| `Attachments` | `[]Attachment` | File attachments |
| `InlineAttachments` | `[]InlineAttachment` | Embedded images |
| `Headers` | `map[string]string` | Custom headers |
| `Priority` | `int` | Priority (1=high, 3=normal, 5=low) |
| `Tags` | `[]string` | Tracking/categorization tags |
| `Metadata` | `map[string]string` | Custom metadata |
| `SendAt` | `*time.Time` | Scheduled send time |
| `TrackOpens` | `bool` | Enable open tracking |
| `TrackClicks` | `bool` | Enable click tracking |

## SMTP Implementation

### Quick Start

```go
import (
    "context"
    emailpkg "github.com/plaenen/eventstore/pkg/comms/email"
    "github.com/plaenen/eventstore/pkg/comms/email/smtp"
)

// Create sender with functional options
sender := smtp.NewSender(
    smtp.WithHost("smtp.gmail.com"),
    smtp.WithPort(587),
    smtp.WithAuth("user@gmail.com", "app-password"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("user@gmail.com", "My Name"),
)

// Build and send email
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Hello from Go!").
    TextBody("This is a test email.").
    Build()

ctx := context.Background()
if err := sender.Send(ctx, email); err != nil {
    log.Fatal(err)
}
```

### Configuration Options

#### Functional Options Pattern (Recommended)

```go
sender := smtp.NewSender(
    smtp.WithHost("smtp.example.com"),
    smtp.WithPort(587),
    smtp.WithAuth("username", "password"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("sender@example.com", "Sender Name"),
    smtp.WithTimeout(30 * time.Second),
    smtp.WithMaxMessageSize(25 * 1024 * 1024), // 25 MB
)
```

#### Config Struct (Backward Compatible)

```go
config := &smtp.Config{
    Host:        "smtp.example.com",
    Port:        587,
    Username:    "user@example.com",
    Password:    "password",
    UseSTARTTLS: true,
    FromAddress: "user@example.com",
    FromName:    "My App",
    Timeout:     30 * time.Second,
}
sender := smtp.NewSender(config)
```

### Available Options

| Option | Description |
|--------|-------------|
| `WithHost(host)` | SMTP server hostname |
| `WithPort(port)` | SMTP server port |
| `WithAuth(user, pass)` | Authentication credentials |
| `WithTLS()` | Use direct TLS (SMTPS, port 465) |
| `WithSTARTTLS()` | Use STARTTLS upgrade (port 587) |
| `WithTLSConfig(cfg)` | Custom TLS configuration |
| `WithSkipVerify()` | Skip TLS verification (dev only!) |
| `WithTimeout(d)` | Connection timeout |
| `WithKeepAlive(d)` | TCP keep-alive duration |
| `WithFrom(addr, name)` | Default sender address and name |
| `WithMaxMessageSize(n)` | Maximum message size in bytes |

### TLS/Security Options

```go
// SMTPS (TLS from the start, typically port 465)
sender := smtp.NewSender(
    smtp.WithHost("smtp.example.com"),
    smtp.WithPort(465),
    smtp.WithTLS(),
    smtp.WithAuth("user", "pass"),
)

// STARTTLS (upgrade to TLS, typically port 587)
sender := smtp.NewSender(
    smtp.WithHost("smtp.example.com"),
    smtp.WithPort(587),
    smtp.WithSTARTTLS(),
    smtp.WithAuth("user", "pass"),
)

// Development only: skip certificate verification
sender := smtp.NewSender(
    smtp.WithHost("localhost"),
    smtp.WithPort(1025),
    smtp.WithSkipVerify(), // WARNING: Never use in production!
)
```

## Email Builder

The fluent builder pattern makes constructing emails clean and readable:

```go
email := emailpkg.NewEmail().
    From("sender@example.com").
    To("recipient@example.com").
    CC("manager@example.com").
    BCC("audit@example.com").
    ReplyTo("reply@example.com").
    Subject("Project Update").
    TextBody("Plain text version").
    HTMLBody("<html><body><h1>HTML Version</h1></body></html>").
    Attach("report.pdf", "application/pdf", pdfData).
    AttachFile("/path/to/file.txt").
    InlineImage("logo", "logo.png", "image/png", logoData).
    Header("X-Custom-Header", "value").
    Priority(1).
    Tag("notification", "important").
    Meta("campaign_id", "12345").
    TrackOpens().
    TrackClicks().
    Build()
```

## Usage Examples

### Simple Text Email

```go
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Hello").
    TextBody("This is a simple text email.").
    Build()

sender.Send(ctx, email)
```

### HTML Email with Text Fallback

```go
email := emailpkg.NewEmail().
    From("sender@example.com").
    To("recipient@example.com").
    Subject("Welcome!").
    TextBody("Welcome to our service! View this email in HTML for best experience.").
    HTMLBody(`
        <html>
        <body>
            <h1>Welcome!</h1>
            <p>Thanks for signing up.</p>
        </body>
        </html>
    `).
    Build()
```

### Email with Attachments

```go
// Attach from bytes
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Invoice Attached").
    TextBody("Please find your invoice attached.").
    Attach("invoice.pdf", "application/pdf", pdfBytes).
    Build()

// Attach from file path
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Report").
    TextBody("See attached report.").
    AttachFile("/path/to/report.xlsx").
    Build()
```

### Email with Inline Images

```go
logoData, _ := os.ReadFile("logo.png")

email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Newsletter").
    HTMLBody(`
        <html>
        <body>
            <img src="cid:company-logo" alt="Logo">
            <h1>Monthly Newsletter</h1>
        </body>
        </html>
    `).
    InlineImage("company-logo", "logo.png", "image/png", logoData).
    Build()
```

### Batch Sending

For sending multiple emails efficiently (reuses single SMTP connection):

```go
emails := make([]*emailpkg.Email, 0, 100)
for _, user := range users {
    email := emailpkg.NewEmail().
        To(user.Email).
        Subject("Welcome!").
        TextBody(fmt.Sprintf("Hello %s!", user.Name)).
        Build()
    emails = append(emails, email)
}

// Send all in one connection
if err := sender.SendBatch(ctx, emails); err != nil {
    log.Printf("Batch send failed: %v", err)
}
```

### High Priority Email

```go
email := emailpkg.NewEmail().
    To("admin@example.com").
    Subject("URGENT: Server Alert").
    TextBody("Production server is down!").
    Priority(1). // 1 = highest priority
    Build()
```

### Custom Headers

```go
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Custom Headers").
    TextBody("Email with custom headers.").
    Header("X-Mailer", "MyApp/1.0").
    Header("X-Campaign-ID", "summer-sale-2024").
    Header("List-Unsubscribe", "<mailto:unsubscribe@example.com>").
    Build()
```

### Testing Connection

```go
sender := smtp.NewSender(
    smtp.WithHost("smtp.gmail.com"),
    smtp.WithPort(587),
    smtp.WithAuth("user@gmail.com", "app-password"),
    smtp.WithSTARTTLS(),
)

ctx := context.Background()
if err := sender.TestConnection(ctx); err != nil {
    log.Fatalf("Connection failed: %v", err)
}
fmt.Println("Connection successful!")
```

## Provider Configuration

### Gmail

```go
// Enable 2FA and create an App Password at:
// https://myaccount.google.com/apppasswords

sender := smtp.NewSender(
    smtp.WithHost("smtp.gmail.com"),
    smtp.WithPort(587),
    smtp.WithAuth("your-email@gmail.com", "your-16-char-app-password"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("your-email@gmail.com", "Your Name"),
)
```

### Office 365

```go
sender := smtp.NewSender(
    smtp.WithHost("smtp.office365.com"),
    smtp.WithPort(587),
    smtp.WithAuth("your-email@company.com", "your-password"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("your-email@company.com", "Your Name"),
)
```

### Amazon SES

```go
sender := smtp.NewSender(
    smtp.WithHost("email-smtp.us-east-1.amazonaws.com"),
    smtp.WithPort(587),
    smtp.WithAuth("SMTP_USERNAME", "SMTP_PASSWORD"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("verified-sender@yourdomain.com"),
)
```

### SendGrid

```go
sender := smtp.NewSender(
    smtp.WithHost("smtp.sendgrid.net"),
    smtp.WithPort(587),
    smtp.WithAuth("apikey", "YOUR_SENDGRID_API_KEY"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("sender@yourdomain.com"),
)
```

### Mailgun

```go
sender := smtp.NewSender(
    smtp.WithHost("smtp.mailgun.org"),
    smtp.WithPort(587),
    smtp.WithAuth("postmaster@yourdomain.mailgun.org", "your-mailgun-password"),
    smtp.WithSTARTTLS(),
    smtp.WithFrom("sender@yourdomain.com"),
)
```

## Testing

### Unit Tests

```bash
go test ./pkg/comms/email/smtp/...
```

### Integration Tests (with Mailpit)

The package includes comprehensive integration tests using [Mailpit](https://github.com/axllent/mailpit) via testcontainers:

```bash
go test -tags=integration ./pkg/comms/email/smtp/...
```

Integration tests verify:
- Basic email sending
- HTML email with multipart content
- Attachments (send and download verification)
- Batch sending performance
- All configuration options
- Headers and priority settings

### Local Development with Mailpit

For local development, run Mailpit:

```bash
docker run -d -p 1025:1025 -p 8025:8025 axllent/mailpit
```

Then configure your sender:

```go
sender := smtp.NewSender(
    smtp.WithHost("localhost"),
    smtp.WithPort(1025),
    smtp.WithSkipVerify(),
)
```

View emails at http://localhost:8025

## Error Handling

The sender validates emails before sending:

```go
err := sender.Send(ctx, email)
if err != nil {
    switch {
    case strings.Contains(err.Error(), "validation failed"):
        // Invalid email structure
    case strings.Contains(err.Error(), "authentication failed"):
        // Wrong credentials
    case strings.Contains(err.Error(), "failed to dial"):
        // Network/connection issue
    case strings.Contains(err.Error(), "context error"):
        // Context canceled or deadline exceeded
    default:
        // Other SMTP error
    }
}
```

## Context Support

All operations support context for cancellation and timeouts:

```go
// With timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := sender.Send(ctx, email); err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        log.Println("Send timed out")
    }
}

// With cancellation
ctx, cancel := context.WithCancel(context.Background())
go func() {
    time.Sleep(5 * time.Second)
    cancel() // Cancel if taking too long
}()

sender.SendBatch(ctx, emails)
```

## Thread Safety

The `SMTPSender` is safe for concurrent use. Each `Send` and `SendBatch` call creates its own connection.

## Future Providers

The architecture supports adding additional email providers:

- **Transactional APIs**: SendGrid, Mailgun, Amazon SES, Postmark
- **Cloud providers**: AWS SES, Google Cloud, Azure Communication Services
- **Self-hosted**: Postal, Mailcow

Implement the `EmailSender` interface to add new providers.
