package datastarx

import (
	"testing"

	"github.com/plaenen/eventstore/pkg/validators"
	"github.com/stretchr/testify/assert"
)

func TestToUserFeedback_SingleFieldMultipleFeedbacks(t *testing.T) {
	// Test case: Single field with multiple validation messages
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "email",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "",
					Message:         "Email is required",
					SuggestedAction: "Please enter your email address",
					ValidationCode:  validators.ValidationCodeRequired,
				},
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "invalid-email",
					Message:         "Email format is invalid",
					SuggestedAction: "Please enter a valid email address",
					ValidationCode:  validators.ValidationCodeInvalid,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Verify overall message type and message
	assert.Equal(t, MessageTypeError, userFeedback.messageType)
	assert.Equal(t, "Validation failed", userFeedback.message)

	// Verify multiple feedbacks exist for the email field
	emailFeedbacks := userFeedback.mapFieldFeedback["email"]
	assert.Len(t, emailFeedbacks, 2)

	// Verify first feedback
	assert.Equal(t, "email", emailFeedbacks[0].FieldName)
	assert.Equal(t, "", emailFeedbacks[0].FieldValue)
	assert.Equal(t, "Email is required", emailFeedbacks[0].ValidationMessage)
	assert.True(t, emailFeedbacks[0].NeedsUserAction)
	assert.Equal(t, "Please enter your email address", emailFeedbacks[0].SuggestedAction)

	// Verify second feedback
	assert.Equal(t, "email", emailFeedbacks[1].FieldName)
	assert.Equal(t, "invalid-email", emailFeedbacks[1].FieldValue)
	assert.Equal(t, "Email format is invalid", emailFeedbacks[1].ValidationMessage)
	assert.True(t, emailFeedbacks[1].NeedsUserAction)
	assert.Equal(t, "Please enter a valid email address", emailFeedbacks[1].SuggestedAction)
}

func TestToUserFeedback_MultipleFields(t *testing.T) {
	// Test case: Multiple fields with validation issues
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "email",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "",
					Message:         "Email is required",
					SuggestedAction: "Please enter your email address",
					ValidationCode:  validators.ValidationCodeRequired,
				},
			},
		},
		{
			FieldName: "password",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "password",
					Value:           "123",
					Message:         "Password is too short",
					SuggestedAction: "Please enter a password with at least 8 characters",
					ValidationCode:  validators.ValidationCodeInvalid,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Verify overall message type and message for multiple fields
	assert.Equal(t, MessageTypeError, userFeedback.messageType)
	assert.Equal(t, "Validation failed for 2 fields", userFeedback.message)

	// Verify both fields have feedback
	assert.Contains(t, userFeedback.mapFieldFeedback, "email")
	assert.Contains(t, userFeedback.mapFieldFeedback, "password")

	// Verify email feedback
	emailFeedbacks := userFeedback.mapFieldFeedback["email"]
	assert.Len(t, emailFeedbacks, 1)
	assert.Equal(t, "Email is required", emailFeedbacks[0].ValidationMessage)

	// Verify password feedback
	passwordFeedbacks := userFeedback.mapFieldFeedback["password"]
	assert.Len(t, passwordFeedbacks, 1)
	assert.Equal(t, "Password is too short", passwordFeedbacks[0].ValidationMessage)
}

func TestToUserFeedback_MixedValidationTypes(t *testing.T) {
	// Test case: Mixed validation types (errors and warnings)
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "email",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "",
					Message:         "Email is required",
					SuggestedAction: "Please enter your email address",
					ValidationCode:  validators.ValidationCodeRequired,
				},
			},
		},
		{
			FieldName: "username",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         true,
					FieldName:       "username",
					Value:           "john_doe",
					Message:         "Username is acceptable but could be improved",
					SuggestedAction: "Consider using a more professional username",
					ValidationCode:  validators.ValidationCodeUnspecified,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Should prioritize error over warning
	assert.Equal(t, MessageTypeError, userFeedback.messageType)
	assert.Equal(t, "Validation failed for 2 fields", userFeedback.message)

	// Verify email feedback (error)
	emailFeedbacks := userFeedback.mapFieldFeedback["email"]
	assert.Len(t, emailFeedbacks, 1)
	assert.True(t, emailFeedbacks[0].NeedsUserAction)

	// Verify username feedback (warning)
	usernameFeedbacks := userFeedback.mapFieldFeedback["username"]
	assert.Len(t, usernameFeedbacks, 1)
	assert.False(t, usernameFeedbacks[0].NeedsUserAction)
}

func TestToUserFeedback_WarningsOnly(t *testing.T) {
	// Test case: Only warnings, no errors
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "username",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         true,
					FieldName:       "username",
					Value:           "john_doe",
					Message:         "Username is acceptable but could be improved",
					SuggestedAction: "Consider using a more professional username",
					ValidationCode:  validators.ValidationCodeUnspecified,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Should use warning message type
	assert.Equal(t, MessageTypeWarning, userFeedback.messageType)
	assert.Equal(t, "Validation warnings", userFeedback.message)

	// Verify username feedback
	usernameFeedbacks := userFeedback.mapFieldFeedback["username"]
	assert.Len(t, usernameFeedbacks, 1)
	assert.False(t, usernameFeedbacks[0].NeedsUserAction)
}

func TestToUserFeedback_SuccessOnly(t *testing.T) {
	// Test case: Only success validations
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "email",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         true,
					FieldName:       "email",
					Value:           "user@example.com",
					Message:         "Email is valid",
					SuggestedAction: "",
					ValidationCode:  validators.ValidationCodeSuccess,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Should use info message type
	assert.Equal(t, MessageTypeInfo, userFeedback.messageType)
	assert.Equal(t, "Validation completed", userFeedback.message)

	// Verify email feedback
	emailFeedbacks := userFeedback.mapFieldFeedback["email"]
	assert.Len(t, emailFeedbacks, 1)
	assert.False(t, emailFeedbacks[0].NeedsUserAction)
}

func TestToUserFeedback_EmptyInput(t *testing.T) {
	// Test case: Empty validation input
	fieldValidations := []validators.FieldValidations{}

	userFeedback := ToUserFeedback(fieldValidations)

	// Should handle empty input gracefully
	assert.Equal(t, MessageTypeInfo, userFeedback.messageType)
	assert.Equal(t, "Validation completed", userFeedback.message)
	assert.Empty(t, userFeedback.mapFieldFeedback)
}

func TestToUserFeedback_ComplexMultipleFeedbacks(t *testing.T) {
	// Test case: Complex scenario with multiple fields and multiple feedbacks per field
	fieldValidations := []validators.FieldValidations{
		{
			FieldName: "email",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "",
					Message:         "Email is required",
					SuggestedAction: "Please enter your email address",
					ValidationCode:  validators.ValidationCodeRequired,
				},
				{
					IsValid:         false,
					FieldName:       "email",
					Value:           "invalid-email",
					Message:         "Email format is invalid",
					SuggestedAction: "Please enter a valid email address",
					ValidationCode:  validators.ValidationCodeInvalid,
				},
				{
					IsValid:         true,
					FieldName:       "email",
					Value:           "user@example.com",
					Message:         "Email domain is trusted",
					SuggestedAction: "",
					ValidationCode:  validators.ValidationCodeSuccess,
				},
			},
		},
		{
			FieldName: "password",
			Validations: []*validators.ValidationResult{
				{
					IsValid:         false,
					FieldName:       "password",
					Value:           "123",
					Message:         "Password is too short",
					SuggestedAction: "Please enter a password with at least 8 characters",
					ValidationCode:  validators.ValidationCodeInvalid,
				},
				{
					IsValid:         false,
					FieldName:       "password",
					Value:           "123",
					Message:         "Password must contain special characters",
					SuggestedAction: "Please include at least one special character",
					ValidationCode:  validators.ValidationCodeInvalid,
				},
			},
		},
	}

	userFeedback := ToUserFeedback(fieldValidations)

	// Verify overall message type (should be error due to required/invalid codes)
	assert.Equal(t, MessageTypeError, userFeedback.messageType)
	assert.Equal(t, "Validation failed for 2 fields", userFeedback.message)

	// Verify email field has 3 feedbacks
	emailFeedbacks := userFeedback.mapFieldFeedback["email"]
	assert.Len(t, emailFeedbacks, 3)

	// Verify password field has 2 feedbacks
	passwordFeedbacks := userFeedback.mapFieldFeedback["password"]
	assert.Len(t, passwordFeedbacks, 2)

	// Verify user action count
	assert.Equal(t, 4, userFeedback.GetUserActionCount()) // 2 from email (required + invalid) + 2 from password (both invalid)
}
