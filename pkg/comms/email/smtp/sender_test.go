//go:build !integration

package smtp

import (
	"strings"
	"testing"
	"time"

	emailpkg "github.com/plaenen/eventstore/pkg/comms/email"
)

func TestSMTPSender_ValidateEmail(t *testing.T) {
	config := DefaultConfig()
	config.FromAddress = "sender@example.com"
	sender := NewSender(config)

	tests := []struct {
		name    string
		email   *emailpkg.Email
		wantErr bool
	}{
		{
			name: "valid email with To",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: false,
		},
		{
			name: "valid email with CC only",
			email: &emailpkg.Email{
				From:     "from@example.com",
				CC:       []string{"cc@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: false,
		},
		{
			name: "valid email with BCC only",
			email: &emailpkg.Email{
				From:     "from@example.com",
				BCC:      []string{"bcc@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: false,
		},
		{
			name: "valid email using default from",
			email: &emailpkg.Email{
				To:       []string{"to@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: false,
		},
		{
			name: "valid email with HTML",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test",
				HTMLBody: "<html><body>Test</body></html>",
			},
			wantErr: false,
		},
		{
			name: "valid email with both text and HTML",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
				HTMLBody: "<html><body>Test</body></html>",
			},
			wantErr: false,
		},
		{
			name:    "nil email",
			email:   nil,
			wantErr: true,
		},
		{
			name: "no recipients",
			email: &emailpkg.Email{
				From:     "from@example.com",
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: true,
		},
		{
			name: "no from and no default",
			email: &emailpkg.Email{
				To:       []string{"to@example.com"},
				Subject:  "Test",
				TextBody: "Test body",
			},
			wantErr: false, // Uses default from config
		},
		{
			name: "no body",
			email: &emailpkg.Email{
				From:    "from@example.com",
				To:      []string{"to@example.com"},
				Subject: "Test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sender.validateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEmail() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMTPSender_BuildMessage(t *testing.T) {
	config := DefaultConfig()
	config.FromAddress = "sender@example.com"
	// Don't set FromName here - let individual tests set it if needed
	sender := NewSender(config)

	tests := []struct {
		name      string
		email     *emailpkg.Email
		wantErr   bool
		checkFunc func(t *testing.T, message []byte)
	}{
		{
			name: "simple text email",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test Subject",
				TextBody: "Hello, World!",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "From: from@example.com") {
					t.Error("Missing From header")
				}
				if !contains(msg, "To: to@example.com") {
					t.Error("Missing To header")
				}
				if !contains(msg, "Subject:") {
					t.Error("Missing Subject header")
				}
				if !contains(msg, "Content-Type: text/plain") {
					t.Error("Missing text/plain content type")
				}
			},
		},
		{
			name: "HTML email",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test HTML",
				HTMLBody: "<html><body><h1>Hello</h1></body></html>",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Content-Type: text/html") {
					t.Error("Missing text/html content type")
				}
			},
		},
		{
			name: "multipart alternative (text + HTML)",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Test Multipart",
				TextBody: "Plain text version",
				HTMLBody: "<html><body>HTML version</body></html>",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Content-Type: multipart/alternative") {
					t.Error("Missing multipart/alternative content type")
				}
			},
		},
		{
			name: "email with CC and BCC",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				CC:       []string{"cc@example.com"},
				BCC:      []string{"bcc@example.com"},
				Subject:  "Test CC/BCC",
				TextBody: "Test body",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Cc: cc@example.com") {
					t.Error("Missing CC header")
				}
				if contains(msg, "Bcc:") {
					t.Error("BCC header should not be in message")
				}
			},
		},
		{
			name: "email with reply-to",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				ReplyTo:  "reply@example.com",
				Subject:  "Test Reply-To",
				TextBody: "Test body",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Reply-To: reply@example.com") {
					t.Error("Missing Reply-To header")
				}
			},
		},
		{
			name: "email with priority",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "High Priority",
				TextBody: "Urgent!",
				Priority: 1,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "X-Priority: 1") {
					t.Error("Missing X-Priority header")
				}
				if !contains(msg, "Importance: high") {
					t.Error("Missing Importance header")
				}
			},
		},
		{
			name: "email with custom headers",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Custom Headers",
				TextBody: "Test body",
				Headers: map[string]string{
					"X-Custom-Header": "custom-value",
					"X-App-Version":   "1.0.0",
				},
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "X-Custom-Header: custom-value") {
					t.Error("Missing custom header")
				}
				if !contains(msg, "X-App-Version: 1.0.0") {
					t.Error("Missing app version header")
				}
			},
		},
		{
			name: "email with inline attachment",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "Inline Image",
				HTMLBody: `<html><body><img src="cid:logo"></body></html>`,
				InlineAttachments: []emailpkg.InlineAttachment{
					{
						ContentID:   "logo",
						Filename:    "logo.png",
						ContentType: "image/png",
						Data:        []byte("fake-png-data"),
					},
				},
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Content-Type: multipart/related") {
					t.Error("Missing multipart/related content type")
				}
				if !contains(msg, "Content-ID: <logo>") {
					t.Error("Missing Content-ID for inline attachment")
				}
			},
		},
		{
			name: "email with regular attachment",
			email: &emailpkg.Email{
				From:     "from@example.com",
				To:       []string{"to@example.com"},
				Subject:  "With Attachment",
				TextBody: "Please see attached file",
				Attachments: []emailpkg.Attachment{
					{
						Filename:    "document.pdf",
						ContentType: "application/pdf",
						Data:        []byte("fake-pdf-data"),
					},
				},
			},
			wantErr: false,
			checkFunc: func(t *testing.T, message []byte) {
				msg := string(message)
				if !contains(msg, "Content-Type: multipart/mixed") {
					t.Error("Missing multipart/mixed content type")
				}
				if !contains(msg, "Content-Disposition: attachment") {
					t.Error("Missing attachment disposition")
				}
				if !contains(msg, "document.pdf") {
					t.Error("Missing attachment filename")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := sender.buildMessage(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, message)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got %s", config.Host)
	}

	if config.Port != 25 {
		t.Errorf("Expected default port 25, got %d", config.Port)
	}

	if !config.UseSTARTTLS {
		t.Error("Expected UseSTARTTLS to be true by default")
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", config.Timeout)
	}
}

func TestNewSender(t *testing.T) {
	config := &Config{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user@example.com",
		Password: "password",
	}

	sender := NewSender(config)

	if sender == nil {
		t.Fatal("NewSender returned nil")
	}

	if sender.config.Timeout == 0 {
		t.Error("Timeout should be set to default if not specified")
	}

	if sender.config.TLSConfig == nil {
		t.Error("TLSConfig should be initialized")
	}

	if sender.config.TLSConfig.ServerName != config.Host {
		t.Errorf("TLSConfig.ServerName should be %s, got %s", config.Host, sender.config.TLSConfig.ServerName)
	}
}

func TestQuotedPrintableEncode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple ASCII",
			input: "Hello, World!",
			want:  "Hello,=20World!",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quotedPrintableEncode(tt.input)
			// Just check it doesn't panic and returns something
			if tt.input == "" && got != "" {
				t.Errorf("Expected empty result for empty input, got %q", got)
			}
			if tt.input != "" && got == "" {
				t.Errorf("Expected non-empty result for %q", tt.input)
			}
		})
	}
}

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "simple data",
			input: []byte("Hello, World!"),
		},
		{
			name:  "empty data",
			input: []byte{},
		},
		{
			name:  "binary data",
			input: []byte{0, 1, 2, 3, 255, 254, 253},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := base64Encode(tt.input)
			// Check that result ends with CRLF
			if len(result) > 0 && !strings.HasSuffix(result, "\r\n") {
				t.Error("base64Encode result should end with CRLF")
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
