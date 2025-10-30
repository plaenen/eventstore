//go:build integration
// +build integration

package smtp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	emailpkg "github.com/plaenen/eventstore/pkg/email"
	"github.com/plaenen/eventstore/pkg/email/smtp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupMailpit starts a Mailpit container for testing
func setupMailpit(t *testing.T) (smtpHost string, smtpPort string, webUI string, cleanup func()) {
	ctx := context.Background()

	// Define Mailpit container
	req := testcontainers.ContainerRequest{
		Image:        "axllent/mailpit:latest",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("1025/tcp"),
			wait.ForListeningPort("8025/tcp"),
			wait.ForLog("accessible via"),
		),
		Env: map[string]string{
			"MP_SMTP_AUTH_ACCEPT_ANY":    "1",
			"MP_SMTP_AUTH_ALLOW_INSECURE": "1",
		},
	}

	// Start container
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start Mailpit container: %v", err)
	}

	// Get SMTP port
	smtpMappedPort, err := container.MappedPort(ctx, "1025")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get SMTP port: %v", err)
	}

	// Get Web UI port
	webMappedPort, err := container.MappedPort(ctx, "8025")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get Web UI port: %v", err)
	}

	// Get host
	smtpHost, err = container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container host: %v", err)
	}

	smtpPort = smtpMappedPort.Port()
	tempWebPort := webMappedPort.Port()
	webUI = fmt.Sprintf("http://%s:%s", smtpHost, tempWebPort)

	t.Logf("📧 Mailpit started:")
	t.Logf("   SMTP: %s:%s", smtpHost, smtpPort)
	t.Logf("   Web UI: %s", webUI)

	cleanup = func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	return
}

func TestMailpitIntegration_BasicEmail(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	// Create Mailpit API client
	apiClient := NewMailpitClient(webUI)

	sender := smtp.NewSender(
		smtp.WithHost(host),
		smtp.WithPort(mustAtoi(port)),
		smtp.WithSkipVerify(),
	)

	// Test connection
	ctx := context.Background()
	if err := sender.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}

	// Send email
	subject := "Testcontainers - Basic Email"
	bodyText := "This email was sent using testcontainers!"

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject(subject).
		TextBody(bodyText).
		Build()

	if err := sender.Send(ctx, email); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify email was received
	msg, err := apiClient.WaitForMessage(subject, 5*time.Second)
	if err != nil {
		t.Fatalf("Email not received: %v", err)
	}

	// Verify content
	if msg.Subject != subject {
		t.Errorf("Subject mismatch: got %q, want %q", msg.Subject, subject)
	}

	if !strings.Contains(msg.Text, bodyText) {
		t.Errorf("Body text not found in message")
	}

	if msg.From.Address != "sender@example.com" {
		t.Errorf("From address mismatch: got %q, want %q", msg.From.Address, "sender@example.com")
	}

	if !apiClient.VerifyRecipient(msg, "recipient@example.com") {
		t.Error("Recipient not found in message")
	}

	t.Log("✅ Email sent and verified successfully")
	t.Logf("📧 View at: %s", webUI)
}

func TestMailpitIntegration_HTMLEmail(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	sender := smtp.NewSender(
		smtp.WithHost(host),
		smtp.WithPort(mustAtoi(port)),
		smtp.WithSkipVerify(),
	)

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject("Testcontainers - HTML Email").
		TextBody("Plain text version").
		HTMLBody(`
			<html>
			<body style="font-family: Arial;">
				<h1 style="color: #2c3e50;">Hello from Testcontainers!</h1>
				<p>This is a <strong>styled</strong> HTML email.</p>
			</body>
			</html>
		`).
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	t.Log("✅ HTML email sent")
	t.Logf("📧 View at: %s", webUI)
}

func TestMailpitIntegration_WithAttachment(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	apiClient := NewMailpitClient(webUI)

	sender := smtp.NewSender(
		smtp.WithHost(host),
		smtp.WithPort(mustAtoi(port)),
		smtp.WithSkipVerify(),
	)

	subject := "Testcontainers - With Attachment"
	testData := []byte("Invoice #12345\nAmount: $500.00")

	email := emailpkg.NewEmail().
		From("sender@example.com").
		To("recipient@example.com").
		Subject(subject).
		TextBody("Please find invoice attached.").
		Attach("invoice.txt", "text/plain", testData).
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify email was received
	msg, err := apiClient.WaitForMessage(subject, 5*time.Second)
	if err != nil {
		t.Fatalf("Email not received: %v", err)
	}

	// Verify attachment
	if len(msg.Attachments) == 0 {
		t.Fatal("No attachments found")
	}

	att := msg.Attachments[0]
	if att.FileName != "invoice.txt" {
		t.Errorf("Attachment filename mismatch: got %q, want %q", att.FileName, "invoice.txt")
	}

	// Download and verify attachment content
	attData, err := apiClient.GetAttachment(msg.ID, att.PartID)
	if err != nil {
		t.Fatalf("Failed to download attachment: %v", err)
	}

	if !strings.Contains(string(attData), "Invoice #12345") {
		t.Error("Attachment content mismatch")
	}

	t.Log("✅ Email with attachment sent and verified")
	t.Logf("📧 View and download attachment at: %s", webUI)
}

func TestMailpitIntegration_BatchSend(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	sender := smtp.NewSender(
		smtp.WithHost(host),
		smtp.WithPort(mustAtoi(port)),
		smtp.WithSkipVerify(),
	)

	// Create 10 emails
	emails := make([]*emailpkg.Email, 10)
	for i := 0; i < 10; i++ {
		emails[i] = emailpkg.NewEmail().
			From("sender@example.com").
			To(fmt.Sprintf("user%d@example.com", i+1)).
			Subject(fmt.Sprintf("Batch Email #%d", i+1)).
			TextBody(fmt.Sprintf("This is batch email number %d.", i+1)).
			Build()
	}

	ctx := context.Background()
	start := time.Now()
	if err := sender.SendBatch(ctx, emails); err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}
	duration := time.Since(start)

	t.Logf("✅ Sent %d emails in %v (%.2f emails/sec)",
		len(emails), duration, float64(len(emails))/duration.Seconds())
	t.Logf("📧 View all emails at: %s", webUI)
}

func TestMailpitIntegration_OptionsPattern(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	// Test various configuration options
	tests := []struct {
		name    string
		options []interface{}
	}{
		{
			name: "with authentication",
			options: []interface{}{
				smtp.WithHost(host),
				smtp.WithPort(mustAtoi(port)),
				smtp.WithAuth("user@example.com", "password"),
				smtp.WithSkipVerify(),
			},
		},
		{
			name: "with default sender",
			options: []interface{}{
				smtp.WithHost(host),
				smtp.WithPort(mustAtoi(port)),
				smtp.WithFrom("default@example.com", "Default Sender"),
				smtp.WithSkipVerify(),
			},
		},
		{
			name: "with timeout",
			options: []interface{}{
				smtp.WithHost(host),
				smtp.WithPort(mustAtoi(port)),
				smtp.WithTimeout(30 * time.Second),
				smtp.WithSkipVerify(),
			},
		},
		{
			name: "with message size limit",
			options: []interface{}{
				smtp.WithHost(host),
				smtp.WithPort(mustAtoi(port)),
				smtp.WithMaxMessageSize(10 * 1024 * 1024), // 10 MB
				smtp.WithSkipVerify(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := smtp.NewSender(tt.options...)

			email := emailpkg.NewEmail().
				From("test@example.com").
				To("recipient@example.com").
				Subject(fmt.Sprintf("Options Test - %s", tt.name)).
				TextBody("Testing configuration options").
				Build()

			ctx := context.Background()
			if err := sender.Send(ctx, email); err != nil {
				t.Fatalf("Send failed: %v", err)
			}

			t.Log("✅ Email sent with custom options")
		})
	}

	t.Logf("📧 View all test emails at: %s", webUI)
}

// Helper function
func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// TestMailpitIntegration_ComprehensiveVerification tests all features with full API verification
func TestMailpitIntegration_ComprehensiveVerification(t *testing.T) {
	host, port, webUI, cleanup := setupMailpit(t)
	defer cleanup()

	apiClient := NewMailpitClient(webUI)

	sender := smtp.NewSender(
		smtp.WithHost(host),
		smtp.WithPort(mustAtoi(port)),
		smtp.WithSkipVerify(),
		smtp.WithFrom("default@example.com", "Default Sender"),
	)

	subject := "Comprehensive Test - All Features"
	testData := []byte("Test document content")

	email := emailpkg.NewEmail().
		To("primary@example.com").
		CC("cc1@example.com", "cc2@example.com").
		BCC("bcc@example.com").
		ReplyTo("reply@example.com").
		Subject(subject).
		TextBody("Plain text version of the email").
		HTMLBody("<html><body><h1>HTML Version</h1><p>Testing all features</p></body></html>").
		Attach("test.txt", "text/plain", testData).
		Header("X-Custom-Header", "test-value").
		Header("X-Test-ID", "12345").
		Priority(1). // High priority
		Build()

	ctx := context.Background()
	if err := sender.Send(ctx, email); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Wait for message to arrive
	msg, err := apiClient.WaitForMessage(subject, 5*time.Second)
	if err != nil {
		t.Fatalf("Email not received: %v", err)
	}

	// Verify From (should use default from options)
	if msg.From.Address != "default@example.com" {
		t.Errorf("From address mismatch: got %q, want %q", msg.From.Address, "default@example.com")
	}
	if msg.From.Name != "Default Sender" {
		t.Errorf("From name mismatch: got %q, want %q", msg.From.Name, "Default Sender")
	}

	// Verify To recipients
	if !apiClient.VerifyRecipient(msg, "primary@example.com") {
		t.Error("Primary recipient not found")
	}

	// Verify CC recipients
	if len(msg.Cc) != 2 {
		t.Errorf("CC count mismatch: got %d, want 2", len(msg.Cc))
	}
	ccEmails := make(map[string]bool)
	for _, cc := range msg.Cc {
		ccEmails[cc.Address] = true
	}
	if !ccEmails["cc1@example.com"] || !ccEmails["cc2@example.com"] {
		t.Error("CC recipients mismatch")
	}

	// Note: BCC recipients won't appear in headers (by design)
	// They receive the email but are hidden

	// Verify Reply-To
	if len(msg.ReplyTo) == 0 || msg.ReplyTo[0].Address != "reply@example.com" {
		t.Error("Reply-To not found or incorrect")
	}

	// Verify both text and HTML content
	if !strings.Contains(msg.Text, "Plain text version") {
		t.Error("Text body content missing")
	}
	if !strings.Contains(msg.HTML, "<h1>HTML Version</h1>") {
		t.Error("HTML body content missing")
	}

	// Verify attachment
	if len(msg.Attachments) == 0 {
		t.Fatal("No attachments found")
	}
	if msg.Attachments[0].FileName != "test.txt" {
		t.Errorf("Attachment filename mismatch: got %q", msg.Attachments[0].FileName)
	}

	// Verify custom headers
	if !apiClient.VerifyHeader(msg, "X-Custom-Header", "test-value") {
		t.Error("Custom header X-Custom-Header not found or incorrect")
	}
	if !apiClient.VerifyHeader(msg, "X-Test-ID", "12345") {
		t.Error("Custom header X-Test-ID not found or incorrect")
	}

	// Verify priority headers
	if !apiClient.VerifyHeader(msg, "X-Priority", "1") {
		t.Error("Priority header not found or incorrect")
	}
	if !apiClient.VerifyHeader(msg, "Importance", "high") {
		t.Error("Importance header not found or incorrect")
	}

	// Verify Message-ID exists
	if len(msg.Headers["Message-Id"]) == 0 && len(msg.Headers["Message-ID"]) == 0 {
		t.Error("Message-ID header not found")
	}

	// Verify MIME headers for multipart
	if len(msg.Headers["Mime-Version"]) == 0 && len(msg.Headers["MIME-Version"]) == 0 {
		t.Error("MIME-Version header not found")
	}

	t.Log("✅ All features verified successfully:")
	t.Log("   ✓ From address and name")
	t.Log("   ✓ To recipients")
	t.Log("   ✓ CC recipients")
	t.Log("   ✓ Reply-To")
	t.Log("   ✓ Text and HTML content")
	t.Log("   ✓ Attachment")
	t.Log("   ✓ Custom headers")
	t.Log("   ✓ Priority headers")
	t.Log("   ✓ MIME headers")
	t.Logf("📧 View at: %s", webUI)
}
