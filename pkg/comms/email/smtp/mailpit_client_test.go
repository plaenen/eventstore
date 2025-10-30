//go:build integration
// +build integration

package smtp_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MailpitClient provides access to Mailpit's API for test verification
type MailpitClient struct {
	baseURL string
	client  *http.Client
}

// NewMailpitClient creates a new Mailpit API client
func NewMailpitClient(baseURL string) *MailpitClient {
	return &MailpitClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Message represents a message from Mailpit API
type Message struct {
	ID          string                 `json:"ID"`
	From        *Contact               `json:"From"`
	To          []*Contact             `json:"To"`
	Cc          []*Contact             `json:"Cc"`
	Bcc         []*Contact             `json:"Bcc"`
	ReplyTo     []*Contact             `json:"ReplyTo"`
	Subject     string                 `json:"Subject"`
	Date        time.Time              `json:"Date"`
	Text        string                 `json:"Text"`
	HTML        string                 `json:"HTML"`
	Attachments []Attachment           `json:"Attachments"`
	Size        int                    `json:"Size"`
	Headers     map[string][]string    `json:"Header"`
	Tags        []string               `json:"Tags"`
}

// Contact represents an email contact
type Contact struct {
	Name    string `json:"Name"`
	Address string `json:"Address"`
}

// Attachment represents an email attachment
type Attachment struct {
	PartID      string `json:"PartID"`
	FileName    string `json:"FileName"`
	ContentType string `json:"ContentType"`
	Size        int    `json:"Size"`
}

// MessagesResponse represents the response from /api/v1/messages
type MessagesResponse struct{
	Total        int        `json:"total"`
	Unread       int        `json:"unread"`
	Count        int        `json:"count"`
	Messages     []*Message `json:"messages"`
	MessagesCount int       `json:"messages_count"`
}

// GetMessages returns all messages from Mailpit
func (c *MailpitClient) GetMessages() ([]*Message, error) {
	resp, err := c.client.Get(c.baseURL + "/api/v1/messages")
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result MessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Messages, nil
}

// GetMessage returns a specific message by ID
func (c *MailpitClient) GetMessage(id string) (*Message, error) {
	resp, err := c.client.Get(fmt.Sprintf("%s/api/v1/message/%s", c.baseURL, id))
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &msg, nil
}

// FindMessageBySubject finds the first message with the given subject
func (c *MailpitClient) FindMessageBySubject(subject string) (*Message, error) {
	messages, err := c.GetMessages()
	if err != nil {
		return nil, err
	}

	for _, msg := range messages {
		if msg.Subject == subject {
			// Get full message details
			return c.GetMessage(msg.ID)
		}
	}

	return nil, fmt.Errorf("no message found with subject: %s", subject)
}

// FindMessagesByRecipient finds all messages sent to the given recipient
func (c *MailpitClient) FindMessagesByRecipient(email string) ([]*Message, error) {
	messages, err := c.GetMessages()
	if err != nil {
		return nil, err
	}

	var result []*Message
	for _, msg := range messages {
		for _, to := range msg.To {
			if to.Address == email {
				fullMsg, err := c.GetMessage(msg.ID)
				if err != nil {
					return nil, err
				}
				result = append(result, fullMsg)
				break
			}
		}
	}

	return result, nil
}

// WaitForMessage waits for a message with the given subject to arrive
func (c *MailpitClient) WaitForMessage(subject string, timeout time.Duration) (*Message, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		msg, err := c.FindMessageBySubject(subject)
		if err == nil {
			return msg, nil
		}

		<-ticker.C
	}

	return nil, fmt.Errorf("timeout waiting for message with subject: %s", subject)
}

// GetMessageCount returns the total number of messages
func (c *MailpitClient) GetMessageCount() (int, error) {
	resp, err := c.client.Get(c.baseURL + "/api/v1/messages")
	if err != nil {
		return 0, fmt.Errorf("failed to get messages: %w", err)
	}
	defer resp.Body.Close()

	var result MessagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Total, nil
}

// DeleteAll deletes all messages
func (c *MailpitClient) DeleteAll() error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/v1/messages", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetAttachment downloads an attachment from a message
func (c *MailpitClient) GetAttachment(messageID, partID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/message/%s/part/%s", c.baseURL, messageID, partID)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// VerifyHeader checks if a message has a specific header value
func (c *MailpitClient) VerifyHeader(msg *Message, headerName, expectedValue string) bool {
	if values, ok := msg.Headers[headerName]; ok {
		for _, v := range values {
			if v == expectedValue {
				return true
			}
		}
	}
	return false
}

// VerifyRecipient checks if a message was sent to a specific recipient
func (c *MailpitClient) VerifyRecipient(msg *Message, email string) bool {
	// Check To
	for _, to := range msg.To {
		if to.Address == email {
			return true
		}
	}

	// Check Cc
	for _, cc := range msg.Cc {
		if cc.Address == email {
			return true
		}
	}

	// Check Bcc (if captured)
	for _, bcc := range msg.Bcc {
		if bcc.Address == email {
			return true
		}
	}

	return false
}
