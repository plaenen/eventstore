package validators

import (
	"fmt"

	"github.com/plaenen/eventstore/pkg/password"
)

func ValidatePassword(fieldName string, value string) *ValidationResult {
	userFriendlyName := ToUserFriendlyName(fieldName)

	if len(value) == 0 {
		defaultOptions := []ValidationOption{
			WithValue(MaskPassword(value)),
			WithMessage(fmt.Sprintf("%s is required", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a valid %s.", userFriendlyName)),
			WithValidationCode(ValidationCodeRequired),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	if password.ValidateStrength(value) != nil {
		defaultOptions := []ValidationOption{
			WithValue(MaskPassword(value)),
			WithMessage(fmt.Sprintf("%s is too weak", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a stronger %s.", userFriendlyName)),
			WithValidationCode(ValidationCodeInvalid),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	defaultOptions := []ValidationOption{
		WithValue(MaskPassword(value)),
		WithValidationCode(ValidationCodeSuccess),
	}
	return NewValidationResult(true, fieldName, defaultOptions...)
}
