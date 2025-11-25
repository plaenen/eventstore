package datastarx

import (
	"fmt"

	"github.com/plaenen/eventstore/pkg/validation"
)

func ToUserFeedback(v []validation.FieldValidations) *UserFeedback {
	// Count total fields and determine overall message type
	totalFields := len(v)
	hasErrors := false
	hasWarnings := false

	// First pass: determine overall message type and count issues
	for _, f := range v {
		for _, val := range f.Validations {
			switch val.ValidationCode {
			case validation.ValidationCodeRequired, validation.ValidationCodeInvalid:
				hasErrors = true
			case validation.ValidationCodeUnspecified:
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
		for _, val := range f.Validations {
			// Determine if this specific validation needs user action
			needsUserAction := val.ValidationCode == validation.ValidationCodeRequired ||
				val.ValidationCode == validation.ValidationCodeInvalid

			userFeedback.SetFeedback(f.FieldName, val.Value, val.Message, needsUserAction, val.SuggestedAction)
		}
	}

	return userFeedback
}
