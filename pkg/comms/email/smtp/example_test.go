//go:build !integration

package smtp

import (
	"context"
	"fmt"
	"log"
	"time"

	emailpkg "github.com/plaenen/eventstore/pkg/email"
)

// Example_basicEmail shows how to send a simple email
func Example_basicEmail() {
	// Configure SMTP
	config := &Config{
		Host:        "smtp.gmail.com",
		Port:        587,
		Username:    "your-email@gmail.com",
		Password:    "your-app-password",
		UseSTARTTLS: true,
		FromAddress: "your-email@gmail.com",
		FromName:    "Your Name",
	}

	sender := NewSender(config)

	// Build email
	email := emailpkg.NewEmail().
		To("recipient@example.com").
		Subject("Hello from SMTP").
		TextBody("This is a plain text email.").
		Build()

	// Send
	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email sent successfully")
}

// Example_htmlEmail shows how to send an HTML email with text fallback
func Example_htmlEmail() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("HTML Email Example").
		TextBody("This is the plain text version.").
		HTMLBody(`
			<html>
			<body>
				<h1>Welcome!</h1>
				<p>This is an <strong>HTML</strong> email.</p>
			</body>
			</html>
		`).
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("HTML email sent")
}

// Example_withAttachment shows how to send an email with attachments
func Example_withAttachment() {
	config := DefaultConfig()
	config.Host = "smtp.example.com"
	config.Username = "user@example.com"
	config.Password = "password"

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Invoice Attached").
		TextBody("Please find the invoice attached.").
		Attach("invoice.pdf", "application/pdf", []byte("fake-pdf-data")).
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email with attachment sent")
}

// Example_withInlineImage shows how to embed images in HTML email
func Example_withInlineImage() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	// Read image data (in real code, read from file)
	logoData := []byte("fake-image-data")

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Email with Logo").
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

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email with inline image sent")
}

// Example_batchSend shows how to send multiple emails efficiently
func Example_batchSend() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
		FromAddress: "sender@example.com",
	}

	sender := NewSender(config)

	// Create multiple emails
	emails := []*emailpkg.Email{
		emailpkg.NewEmail().
			To("user1@example.com").
			Subject("Welcome User 1").
			TextBody("Welcome to our service!").
			Build(),
		emailpkg.NewEmail().
			To("user2@example.com").
			Subject("Welcome User 2").
			TextBody("Welcome to our service!").
			Build(),
		emailpkg.NewEmail().
			To("user3@example.com").
			Subject("Welcome User 3").
			TextBody("Welcome to our service!").
			Build(),
	}

	// Send all in one connection (more efficient)
	ctx := context.Background()
	if err := sender.SendBatch(ctx, emails); err != nil {
		log.Printf("Failed to send batch: %v", err)
		return
	}

	fmt.Printf("Sent %d emails in batch\n", len(emails))
}

// Example_withCCandBCC shows how to use CC and BCC
func Example_withCCandBCC() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("primary@example.com").
		CC("manager@example.com", "colleague@example.com").
		BCC("audit@example.com").
		Subject("Project Update").
		TextBody("Here's the latest project update.").
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email with CC and BCC sent")
}

// Example_highPriority shows how to send a high-priority email
func Example_highPriority() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("URGENT: Server Down").
		TextBody("The production server is down!").
		Priority(1). // 1 = highest priority
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("High-priority email sent")
}

// Example_customHeaders shows how to add custom headers
func Example_customHeaders() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Custom Headers Example").
		TextBody("This email has custom headers.").
		Header("X-Mailer", "MyApp/1.0").
		Header("X-Custom-ID", "12345").
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email with custom headers sent")
}

// Example_withTimeout shows how to use context timeout
func Example_withTimeout() {
	config := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Test Email").
		TextBody("Hello!").
		Build()

	// Create context with 10-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send email: %v", err)
		return
	}

	fmt.Println("Email sent with timeout")
}

// Example_gmailConfiguration shows Gmail-specific configuration
func Example_gmailConfiguration() {
	// For Gmail:
	// 1. Enable 2-factor authentication
	// 2. Generate an "App Password" at https://myaccount.google.com/apppasswords
	// 3. Use the app password (not your regular password)

	config := &Config{
		Host:        "smtp.gmail.com",
		Port:        587,
		Username:    "your-email@gmail.com",
		Password:    "your-16-char-app-password",
		UseSTARTTLS: true,
		FromAddress: "your-email@gmail.com",
		FromName:    "Your Name",
		Timeout:     30 * time.Second,
	}

	sender := NewSender(config)

	// Test connection
	ctx := context.Background()
	if err := sender.TestConnection(ctx); err != nil {
		log.Printf("Connection test failed: %v", err)
		return
	}

	fmt.Println("Gmail connection successful")
}

// Example_office365Configuration shows Office 365 configuration
func Example_office365Configuration() {
	config := &Config{
		Host:        "smtp.office365.com",
		Port:        587,
		Username:    "your-email@company.com",
		Password:    "your-password",
		UseSTARTTLS: true,
		FromAddress: "your-email@company.com",
		FromName:    "Your Name",
		Timeout:     30 * time.Second,
	}

	sender := NewSender(config)

	email := emailpkg.NewEmail().
		To("recipient@example.com").
		Subject("Test from Office 365").
		TextBody("Hello from Office 365!").
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		log.Printf("Failed to send: %v", err)
		return
	}

	fmt.Println("Office 365 email sent")
}

// Example_tlsConfiguration shows TLS/SSL configuration options
func Example_tlsConfiguration() {
	// SMTPS (TLS from the start, port 465)
	configSMTPS := &Config{
		Host:     "smtp.example.com",
		Port:     465,
		Username: "user@example.com",
		Password: "password",
		UseTLS:   true, // Direct TLS
	}
	_ = NewSender(configSMTPS)

	// STARTTLS (upgrade to TLS, port 587)
	configSTARTTLS := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true, // Upgrade to TLS
	}
	_ = NewSender(configSTARTTLS)

	// For development/testing only: Skip certificate verification
	configInsecure := &Config{
		Host:        "smtp.example.com",
		Port:        587,
		Username:    "user@example.com",
		Password:    "password",
		UseSTARTTLS: true,
		SkipVerify:  true, // WARNING: Not for production!
	}
	_ = NewSender(configInsecure)

	fmt.Println("TLS configuration examples created")
}
