package validators

import "fmt"

func ValidateBool(value bool, fieldName string) *ValidationResult {
	userFriendlyName := ToUserFriendlyName(fieldName)

	if !value {
		defaultOptions := []ValidationOption{
			WithValue(fmt.Sprintf("%t", value)),
			WithMessage(fmt.Sprintf("The %s field must be true.", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a valid value for the %s field.", userFriendlyName)),
			WithValidationCode(ValidationCodeRequired),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	defaultOptions := []ValidationOption{
		WithValue(fmt.Sprintf("%t", value)),
		WithValidationCode(ValidationCodeSuccess),
	}
	return NewValidationResult(true, fieldName, defaultOptions...)
}
