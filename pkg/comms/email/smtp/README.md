# SMTP Email Sender

A robust, production-ready SMTP implementation of the `EmailSender` interface with support for all standard email features.

## Features

✅ **Full EmailSender Interface Implementation**
- Send single emails with `Send()`
- Send multiple emails efficiently with `SendBatch()`

✅ **Comprehensive Email Support**
- Plain text and HTML bodies
- Multipart alternative (text + HTML)
- CC and BCC recipients
- Reply-To addresses
- Custom headers
- Email priority (high/normal/low)

✅ **Attachments**
- Regular file attachments
- Inline attachments for embedding images in HTML
- Multiple attachment sources (bytes, reader, file path)
- Automatic content-type detection

✅ **Security**
- TLS/SSL support (SMTPS on port 465)
- STARTTLS support (port 587/25)
- Configurable TLS settings
- SMTP authentication (PLAIN, LOGIN)

✅ **Production Ready**
- Context support for timeouts and cancellation
- Configurable connection pooling
- Message size limits
- Comprehensive error handling
- Test connection method

## Installation

```bash
go get github.com/plaenen/eventstore/pkg/email/smtp
```

## Quick Start

### Basic Email

```go
package main

import (
    "context"
    "log"

    emailpkg "github.com/plaenen/eventstore/pkg/email"
    "github.com/plaenen/eventstore/pkg/email/smtp"
)

func main() {
    // Configure SMTP
    config := &smtp.Config{
        Host:        "smtp.gmail.com",
        Port:        587,
        Username:    "your-email@gmail.com",
        Password:    "your-app-password",
        UseSTARTTLS: true,
        FromAddress: "your-email@gmail.com",
        FromName:    "Your Name",
    }

    sender := smtp.NewSender(config)

    // Build and send email
    email := emailpkg.NewEmail().
        To("recipient@example.com").
        Subject("Hello from SMTP").
        TextBody("This is a plain text email.").
        Build()

    ctx := context.Background()
    if err := sender.Send(ctx, email); err != nil {
        log.Fatalf("Failed to send email: %v", err)
    }

    log.Println("Email sent successfully!")
}
```

## Configuration

### SMTP Config Options

```go
config := &smtp.Config{
    // SMTP server (required)
    Host: "smtp.example.com",
    Port: 587,

    // Authentication (optional)
    Username: "user@example.com",
    Password: "password",

    // TLS Settings
    UseTLS:       false,  // Direct TLS (SMTPS on port 465)
    UseSTARTTLS:  true,   // Upgrade to TLS (port 587)
    SkipVerify:   false,  // Skip cert verification (dev only!)
    TLSConfig:    nil,    // Custom TLS config (optional)

    // Connection settings
    Timeout:   30 * time.Second,
    KeepAlive: 30 * time.Second,

    // Default sender (optional)
    FromAddress: "noreply@example.com",
    FromName:    "My Application",

    // Limits (optional)
    MaxMessageSize: 10 * 1024 * 1024, // 10 MB
}
```

### Common Provider Configurations

#### Gmail

```go
config := &smtp.Config{
    Host:        "smtp.gmail.com",
    Port:        587,
    Username:    "your-email@gmail.com",
    Password:    "your-app-password", // Generate at https://myaccount.google.com/apppasswords
    UseSTARTTLS: true,
    FromAddress: "your-email@gmail.com",
}
```

**Note:** Gmail requires 2-factor authentication and an "App Password" instead of your regular password.

#### Office 365 / Outlook

```go
config := &smtp.Config{
    Host:        "smtp.office365.com",
    Port:        587,
    Username:    "your-email@company.com",
    Password:    "your-password",
    UseSTARTTLS: true,
    FromAddress: "your-email@company.com",
}
```

#### SendGrid

```go
config := &smtp.Config{
    Host:        "smtp.sendgrid.net",
    Port:        587,
    Username:    "apikey",
    Password:    "your-sendgrid-api-key",
    UseSTARTTLS: true,
    FromAddress: "verified-sender@yourdomain.com",
}
```

#### Mailgun

```go
config := &smtp.Config{
    Host:        "smtp.mailgun.org",
    Port:        587,
    Username:    "postmaster@yourdomain.com",
    Password:    "your-smtp-password",
    UseSTARTTLS: true,
    FromAddress: "noreply@yourdomain.com",
}
```

#### AWS SES

```go
config := &smtp.Config{
    Host:        "email-smtp.us-east-1.amazonaws.com",
    Port:        587,
    Username:    "your-smtp-username",
    Password:    "your-smtp-password",
    UseSTARTTLS: true,
    FromAddress: "verified@yourdomain.com",
}
```

## Usage Examples

### HTML Email with Text Fallback

```go
email := emailpkg.NewEmail().
    From("sender@example.com").
    To("recipient@example.com").
    Subject("Welcome!").
    TextBody("Welcome to our service. This is the plain text version.").
    HTMLBody(`
        <html>
        <body>
            <h1>Welcome!</h1>
            <p>Welcome to our <strong>amazing</strong> service.</p>
        </body>
        </html>
    `).
    Build()

err := sender.Send(ctx, email)
```

### Email with Attachments

```go
// From bytes
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Invoice Attached").
    TextBody("Please find the invoice attached.").
    Attach("invoice.pdf", "application/pdf", pdfData).
    Build()

// From file path
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Document Attached").
    TextBody("Please see the attached document.").
    AttachFile("/path/to/document.pdf").
    Build()
```

### Email with Inline Images

```go
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Newsletter").
    HTMLBody(`
        <html>
        <body>
            <img src="cid:company-logo" alt="Logo">
            <p>Welcome to our newsletter!</p>
        </body>
        </html>
    `).
    InlineImage("company-logo", "logo.png", "image/png", logoData).
    Build()
```

### CC and BCC Recipients

```go
email := emailpkg.NewEmail().
    From("sender@example.com").
    To("primary@example.com").
    CC("manager@example.com", "colleague@example.com").
    BCC("audit@example.com"). // Hidden from other recipients
    Subject("Project Update").
    TextBody("Here's the latest update.").
    Build()
```

### High Priority Email

```go
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("URGENT: Action Required").
    TextBody("Please take immediate action.").
    Priority(1). // 1=high, 3=normal, 5=low
    Build()
```

### Custom Headers

```go
email := emailpkg.NewEmail().
    To("recipient@example.com").
    Subject("Custom Headers").
    TextBody("This email has custom headers.").
    Header("X-Mailer", "MyApp/1.0").
    Header("X-Campaign-ID", "summer-2024").
    Header("List-Unsubscribe", "<mailto:unsubscribe@example.com>").
    Build()
```

### Batch Sending (Efficient)

```go
emails := []*emailpkg.Email{
    emailpkg.NewEmail().To("user1@example.com").Subject("Welcome").TextBody("...").Build(),
    emailpkg.NewEmail().To("user2@example.com").Subject("Welcome").TextBody("...").Build(),
    emailpkg.NewEmail().To("user3@example.com").Subject("Welcome").TextBody("...").Build(),
}

// Sends all emails using a single SMTP connection (much faster)
err := sender.SendBatch(ctx, emails)
```

### With Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

err := sender.Send(ctx, email)
if err != nil {
    if ctx.Err() == context.DeadlineExceeded {
        log.Println("Send timeout")
    } else {
        log.Printf("Send failed: %v", err)
    }
}
```

## Testing Connection

```go
config := &smtp.Config{
    Host:        "smtp.example.com",
    Port:        587,
    Username:    "user@example.com",
    Password:    "password",
    UseSTARTTLS: true,
}

sender := smtp.NewSender(config)

// Test connection without sending email
ctx := context.Background()
if err := sender.TestConnection(ctx); err != nil {
    log.Fatalf("Connection failed: %v", err)
}

log.Println("SMTP connection successful!")
```

## TLS/SSL Configuration

### STARTTLS (Recommended for most servers)

```go
config := &smtp.Config{
    Host:        "smtp.example.com",
    Port:        587,  // or 25
    UseSTARTTLS: true, // Upgrade plain connection to TLS
    // ...
}
```

### Direct TLS (SMTPS)

```go
config := &smtp.Config{
    Host:   "smtp.example.com",
    Port:   465,
    UseTLS: true, // Use TLS from the start
    // ...
}
```

### Custom TLS Configuration

```go
import "crypto/tls"

config := &smtp.Config{
    Host:        "smtp.example.com",
    Port:        587,
    UseSTARTTLS: true,
    TLSConfig: &tls.Config{
        ServerName:         "smtp.example.com",
        MinVersion:         tls.VersionTLS12,
        InsecureSkipVerify: false, // Always verify in production!
    },
    // ...
}
```

### Development/Testing Only: Skip Certificate Verification

```go
config := &smtp.Config{
    Host:        "localhost",
    Port:        1025,
    UseSTARTTLS: true,
    SkipVerify:  true, // ⚠️ WARNING: Only for development/testing!
}
```

## Error Handling

```go
err := sender.Send(ctx, email)
if err != nil {
    switch {
    case strings.Contains(err.Error(), "authentication failed"):
        // Handle authentication error
        log.Println("Invalid credentials")

    case strings.Contains(err.Error(), "failed to connect"):
        // Handle connection error
        log.Println("Cannot reach SMTP server")

    case strings.Contains(err.Error(), "validation failed"):
        // Handle validation error
        log.Println("Invalid email configuration")

    case ctx.Err() == context.DeadlineExceeded:
        // Handle timeout
        log.Println("Send timeout")

    default:
        // Other errors
        log.Printf("Send failed: %v", err)
    }
}
```

## Validation

The SMTP sender automatically validates:

- ✅ At least one recipient (To, CC, or BCC)
- ✅ From address (or uses default from config)
- ✅ Either text or HTML body present
- ✅ Message size within limits (if configured)

Example validation error:

```go
email := emailpkg.NewEmail().
    Subject("No recipients").
    TextBody("This will fail validation").
    Build()

err := sender.Send(ctx, email)
// err: "email validation failed: at least one recipient (To, CC, or BCC) is required"
```

## MIME Message Structure

The SMTP sender automatically builds proper MIME messages:

### Simple Text Email
```
Content-Type: text/plain; charset=utf-8
```

### Simple HTML Email
```
Content-Type: text/html; charset=utf-8
```

### Text + HTML (Multipart Alternative)
```
Content-Type: multipart/alternative
├── text/plain
└── text/html
```

### With Inline Images (Multipart Related)
```
Content-Type: multipart/related
├── multipart/alternative
│   ├── text/plain
│   └── text/html
└── image/png (inline, Content-ID: logo)
```

### With Attachments (Multipart Mixed)
```
Content-Type: multipart/mixed
├── multipart/alternative
│   ├── text/plain
│   └── text/html
└── application/pdf (attachment)
```

## Best Practices

### 1. Use Environment Variables for Credentials

```go
config := &smtp.Config{
    Host:     os.Getenv("SMTP_HOST"),
    Port:     587,
    Username: os.Getenv("SMTP_USERNAME"),
    Password: os.Getenv("SMTP_PASSWORD"),
    // ...
}
```

### 2. Always Use Context Timeouts

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := sender.Send(ctx, email)
```

### 3. Use Batch Sending for Multiple Emails

```go
// ✅ Good: Reuses connection
err := sender.SendBatch(ctx, emails)

// ❌ Avoid: Creates new connection for each email
for _, email := range emails {
    sender.Send(ctx, email)
}
```

### 4. Provide Both Text and HTML Bodies

```go
// ✅ Good: Works for all email clients
email := emailpkg.NewEmail().
    TextBody("Plain text version...").
    HTMLBody("<html>...</html>").
    Build()

// ⚠️ Acceptable but not ideal: Some clients may not support HTML
email := emailpkg.NewEmail().
    HTMLBody("<html>...</html>").
    Build()
```

### 5. Handle Errors Gracefully

```go
if err := sender.Send(ctx, email); err != nil {
    // Log error with context
    log.Printf("Failed to send email to %s: %v", email.To, err)

    // Consider retry logic, dead-letter queue, etc.
    // Don't fail the entire request just because email failed
    return nil // Continue processing
}
```

### 6. Test Connection on Startup

```go
func main() {
    sender := smtp.NewSender(config)

    // Verify SMTP configuration on startup
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := sender.TestConnection(ctx); err != nil {
        log.Fatalf("SMTP connection failed: %v", err)
    }

    log.Println("SMTP configured successfully")

    // Start application...
}
```

## Troubleshooting

### "authentication failed"

- Verify username and password are correct
- For Gmail: Use an App Password, not your regular password
- For Office 365: Check if "less secure apps" is enabled (if required)

### "failed to connect"

- Check Host and Port are correct
- Verify firewall allows outbound connections on the SMTP port
- Try telnet test: `telnet smtp.example.com 587`

### "x509: certificate signed by unknown authority"

- Your server's TLS certificate is not trusted
- In production: Add CA certificate to system trust store
- In development: Use `SkipVerify: true` (not recommended for production!)

### "failed to open data writer"

- Message might be too large
- Set `MaxMessageSize` in config
- Split large attachments or use cloud storage links

### Timeout errors

- Increase `Timeout` in config
- Check network latency to SMTP server
- Use context with appropriate timeout

## Performance Considerations

1. **Connection Reuse**: Use `SendBatch()` for multiple emails to reuse connections
2. **Message Size**: Keep messages under 10MB; use links for large files
3. **Concurrent Sending**: Create multiple sender instances for parallel sending
4. **Attachment Optimization**: Compress large files before attaching

## Security Considerations

1. **Always use TLS**: Set `UseTLS` or `UseSTARTTLS` to true
2. **Never skip certificate verification in production**: `SkipVerify: false`
3. **Use environment variables for credentials**: Don't hardcode passwords
4. **Validate user input**: Sanitize email addresses and content
5. **Rate limiting**: Implement rate limits to prevent abuse

## License

Part of the eventsourcing framework.
