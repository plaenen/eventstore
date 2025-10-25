// Common types for DataStar
// This is for dynamic feedback from the client for errors and validation messages
package datastarx

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type MessageType string

const (
	MessageTypeInfo    MessageType = "info"
	MessageTypeWarning MessageType = "warning"
	MessageTypeError   MessageType = "error"
	MessageTypeSuccess MessageType = "success"
)

// UserFeedback defines a feedback data structure which is posted by the client to the server
// This allows the server to patch html elements with the feedback to the client through the datastar go sdk
// The feedback is a map of field names to slices of field feedbacks, allowing multiple feedbacks per field
type UserFeedback struct {
	messageId        string
	message          string
	messageType      MessageType
	mapFieldFeedback map[string][]*FieldFeedback
	timestamp        time.Time
}

type FieldFeedback struct {
	FieldName         string `json:"field_name"`
	FieldValue        string `json:"field_value"`
	ValidationMessage string `json:"validation_message"`
	NeedsUserAction   bool   `json:"needs_user_action"`
	SuggestedAction   string `json:"suggested_action"`
}

func NewUserFeedback(message string, messageType MessageType) *UserFeedback {
	return &UserFeedback{
		messageType:      messageType,
		message:          message,
		messageId:        uuid.New().String(),
		mapFieldFeedback: make(map[string][]*FieldFeedback),
		timestamp:        time.Now(),
	}
}

func (d *UserFeedback) SetFeedback(fieldName string, fieldValue string, validationMessage string, needsUserAction bool, suggestedAction string) {
	// Create new field feedback
	fieldFeedback := &FieldFeedback{
		FieldName:         fieldName,
		FieldValue:        fieldValue,
		ValidationMessage: validationMessage,
		NeedsUserAction:   needsUserAction,
		SuggestedAction:   suggestedAction,
	}

	// Append to existing feedbacks for this field
	d.mapFieldFeedback[fieldName] = append(d.mapFieldFeedback[fieldName], fieldFeedback)
}

func (d *UserFeedback) ToJsonString() (string, error) {
	jsonData, err := d.MarshalJSON()
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func (d *UserFeedback) GetUserActionCount() int {
	count := 0
	for _, fieldFeedbacks := range d.mapFieldFeedback {
		for _, fieldFeedback := range fieldFeedbacks {
			if fieldFeedback.NeedsUserAction {
				count++
			}
		}
	}
	return count
}

// Add custom json marshaller for the FeedbackDefinition struct
func (d *UserFeedback) MarshalJSON() ([]byte, error) {
	jsonMap := map[string]any{
		"message_id":        d.messageId,
		"message":           d.message,
		"message_type":      d.messageType,
		"field_feedback":    d.mapFieldFeedback,
		"timestamp":         d.timestamp.Format(time.RFC3339),
		"user_action_count": d.GetUserActionCount(),
	}
	return json.Marshal(jsonMap)
}

// Add custom json unmarshaller for the UserFeedback struct
func (d *UserFeedback) UnmarshalJSON(data []byte) error {
	var jsonMap map[string]any
	if err := json.Unmarshal(data, &jsonMap); err != nil {
		return err
	}
	d.messageId = jsonMap["message_id"].(string)
	d.message = jsonMap["message"].(string)
	d.messageType = MessageType(jsonMap["message_type"].(string))
	d.mapFieldFeedback = make(map[string][]*FieldFeedback)

	// Handle field_feedback as a map of field names to slices of feedbacks
	if fieldFeedbackMap, ok := jsonMap["field_feedback"].(map[string]any); ok {
		for fieldName, feedbacksValue := range fieldFeedbackMap {
			if feedbacksSlice, ok := feedbacksValue.([]any); ok {
				var fieldFeedbacks []*FieldFeedback
				for _, feedbackValue := range feedbacksSlice {
					if feedbackMap, ok := feedbackValue.(map[string]any); ok {
						fieldFeedback := &FieldFeedback{
							FieldName:         fieldName,
							FieldValue:        feedbackMap["field_value"].(string),
							ValidationMessage: feedbackMap["validation_message"].(string),
							NeedsUserAction:   feedbackMap["needs_user_action"].(bool),
							SuggestedAction:   feedbackMap["suggested_action"].(string),
						}
						fieldFeedbacks = append(fieldFeedbacks, fieldFeedback)
					}
				}
				d.mapFieldFeedback[fieldName] = fieldFeedbacks
			}
		}
	}

	timestamp, err := time.Parse(time.RFC3339, jsonMap["timestamp"].(string))
	if err != nil {
		return err
	}
	d.timestamp = timestamp
	return nil
}

func (d *UserFeedback) MustMarshalJSON() []byte {
	jsonData, err := d.MarshalJSON()
	if err != nil {
		// Create a new user feedback with a default message
		errFeedback := NewUserFeedback("Failed to marshal user feedback", MessageTypeError)
		jsonData, err = errFeedback.MarshalJSON()
		if err != nil {
			panic(err)
		}
	}
	return jsonData
}
