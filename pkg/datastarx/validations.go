package datastarx

import (
	"fmt"

	"github.com/plaenen/eventstore/pkg/validators"
)

func ToUserFeedback(v []validators.FieldValidations) *UserFeedback {
	// Count total fields and determine overall message type
	totalFields := len(v)
	hasErrors := false
	hasWarnings := false

	// First pass: determine overall message type and count issues
	for _, f := range v {
		for _, validation := range f.Validations {
			switch validation.ValidationCode {
			case validators.ValidationCodeRequired, validators.ValidationCodeInvalid:
				hasErrors = true
			case validators.ValidationCodeUnspecified:
				hasWarnings = true
			}
		}
	}

	// Determine overall message type and message
	var messageType MessageType
	var message string

	if hasErrors {
		messageType = MessageTypeError
		if totalFields == 1 {
			message = "Validation failed"
		} else {
			message = fmt.Sprintf("Validation failed for %d fields", totalFields)
		}
	} else if hasWarnings {
		messageType = MessageTypeWarning
		if totalFields == 1 {
			message = "Validation warnings"
		} else {
			message = fmt.Sprintf("Validation warnings for %d fields", totalFields)
		}
	} else {
		messageType = MessageTypeInfo
		message = "Validation completed"
	}

	userFeedback := NewUserFeedback(message, messageType)

	// Second pass: add all feedbacks
	for _, f := range v {
		for _, validation := range f.Validations {
			// Determine if this specific validation needs user action
			needsUserAction := validation.ValidationCode == validators.ValidationCodeRequired ||
				validation.ValidationCode == validators.ValidationCodeInvalid

			userFeedback.SetFeedback(f.FieldName, validation.Value, validation.Message, needsUserAction, validation.SuggestedAction)
		}
	}

	return userFeedback
}
