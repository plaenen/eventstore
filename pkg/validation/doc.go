// Package validation provides both simple and rich validation functions for input validation
// across different architectural layers.
//
// # Overview
//
// This package contains two types of validators designed for different use cases and layers:
//
//  1. Simple validators - Return error, designed for infrastructure/security layer
//  2. Rich validators - Return *ValidationResult, designed for domain/service layer
//
// # Simple Validators
//
// Simple validators are lightweight functions that return a standard error. They are used
// primarily for security-critical validations and infrastructure concerns where you need
// fast, binary validation (valid/invalid) without user-facing messages.
//
// Function names: ValidateXxx (e.g., ValidateEmail, ValidateUUIDv4)
// Return type: error
// Usage: Infrastructure layer, security checks, command validation
//
// Example:
//
//	if err := validation.ValidateEmail(email); err != nil {
//	    return fmt.Errorf("invalid email: %w", err)
//	}
//
// Available simple validators:
//   - ValidateUUIDv4(uuid string) error
//   - ValidateEmail(email string) error
//   - ValidateTenantID(tenantID string) error
//   - ValidateAggregateID(aggregateID string) error
//   - ValidateStringLength(value, fieldName string, minLength, maxLength int) error
//   - ValidateEventType(eventType string) error
//
// # Rich Validators
//
// Rich validators return ValidationResult objects that include user-friendly messages,
// suggested actions, validation codes, and metadata. They are designed for service handlers
// and domain logic where you need to provide detailed, transport-agnostic feedback.
//
// Function names: ValidateXxxField (e.g., ValidateEmailField, ValidatePasswordField)
// Return type: *ValidationResult
// Usage: Service handlers, domain validation, API input validation
//
// Example:
//
//	result := validation.ValidateEmailField("email", userInput)
//	if !result.IsValid {
//	    // Service handler returns transport-agnostic ValidationResult
//	    // BFF layer converts this to transport-specific format (HTTP, SSE, etc.)
//	    return result, nil
//	}
//
// Available rich validators:
//   - ValidateEmailField(fieldName string, value string) *ValidationResult
//   - ValidatePasswordField(fieldName string, value string) *ValidationResult
//   - ValidateBoolField(value bool, fieldName string) *ValidationResult
//   - ValidateStringEmptyField(value string, fieldName string) *ValidationResult
//   - ValidateStringLengthField(value string, fieldName string, minLength, maxLength int) *ValidationResult
//   - ValidateStringPatternField(value string, fieldName string, pattern string, patternName string) *ValidationResult
//
// # Validation Builder
//
// For complex validations involving multiple fields, use ValidationBuilder to collect
// multiple validation results:
//
//	builder := validation.NewValidationBuilder()
//	builder.Add(validation.ValidateEmailField("email", input.Email))
//	builder.Add(validation.ValidatePasswordField("password", input.Password))
//
//	fieldResults := builder.Build()
//	if fieldResults.HasErrors() {
//	    // Handle validation errors
//	    return fieldResults, nil
//	}
//
// # Architectural Layers
//
// The validation package supports a layered architecture:
//
//	┌─────────────────────────────────────────┐
//	│ Frontend (Browser, CLI, etc.)           │
//	└─────────────────┬───────────────────────┘
//	                  │
//	┌─────────────────▼───────────────────────┐
//	│ BFF Layer (datastarx, HTTP handlers)    │
//	│ - Converts ValidationResult → SSE/HTTP  │
//	└─────────────────┬───────────────────────┘
//	                  │
//	┌─────────────────▼───────────────────────┐
//	│ Service Handlers (Domain Layer)         │
//	│ - Uses rich validators (ValidateXxxField)│
//	│ - Returns transport-agnostic results    │
//	└─────────────────┬───────────────────────┘
//	                  │
//	┌─────────────────▼───────────────────────┐
//	│ Infrastructure Layer                    │
//	│ - Uses simple validators (ValidateXxx)  │
//	│ - Security checks, command validation   │
//	└─────────────────────────────────────────┘
//
// This separation ensures that:
//   - Service handlers remain transport-agnostic (no HTTP, SSE, gRPC knowledge)
//   - ValidationResult can be converted to any transport format
//   - Infrastructure layer has fast, lightweight validation
//   - BFF layer handles transport-specific conversions
//
// # Validation Codes
//
// Rich validators use standardized validation codes:
//   - ValidationCodeSuccess: Validation passed
//   - ValidationCodeRequired: Required field is missing
//   - ValidationCodeInvalid: Field value is invalid
//   - ValidationCodeUnspecified: Warning or informational validation
//
// # User-Friendly Names
//
// The ToUserFriendlyName function converts snake_case field names to readable names:
//
//	ToUserFriendlyName("first_name")    // returns "First name"
//	ToUserFriendlyName("email_address") // returns "Email address"
//
// This is automatically used by all rich validators to generate user-friendly messages.
//
// # Security Features
//
// The package includes security helpers:
//   - MaskPassword: Masks password values in validation results
//   - MaskString: Masks sensitive string values
//
// All password validators automatically mask values in validation results.
package validation
