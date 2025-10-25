package validators

import (
	"fmt"

	"github.com/asaskevich/govalidator"
)

func ValidateEmail(fieldName string, value string) *ValidationResult {
	userFriendlyName := ToUserFriendlyName(fieldName)

	if len(value) == 0 {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("%s is required", userFriendlyName)),
			WithSuggestedAction("Please provide a valid email address, e.g., 'name@example.com'."),
			WithValidationCode(ValidationCodeRequired),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	if !govalidator.IsEmail(value) {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("Please enter a valid %s", userFriendlyName)),
			WithSuggestedAction("Please provide a valid email address, e.g., 'name@example.com'."),
			WithValidationCode(ValidationCodeInvalid),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	defaultOptions := []ValidationOption{
		WithValue(value),
		WithValidationCode(ValidationCodeSuccess),
	}
	return NewValidationResult(true, fieldName, defaultOptions...)
}
