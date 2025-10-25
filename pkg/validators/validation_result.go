package validators

import (
	"fmt"

	"github.com/plaenen/eventstore/pkg/eventsourcing"
)

// ValidationCode represents the type of validation result
type ValidationCode string

const (
	ValidationCodeUnspecified ValidationCode = "unspecified"
	ValidationCodeSuccess     ValidationCode = "success"
	ValidationCodeRequired    ValidationCode = "required"
	ValidationCodeInvalid     ValidationCode = "invalid"
)

// ValidationOption defines a function that can customize a ValidationResult
type ValidationOption func(*ValidationResult)

// ValidationResult represents the result of a validation operation
type ValidationResult struct {
	IsValid         bool                   `json:"is_valid"`
	FieldName       string                 `json:"field_name"`
	Value           string                 `json:"value"`
	Message         string                 `json:"message"`
	SuggestedAction string                 `json:"suggested_action"`
	ValidationCode  ValidationCode         `json:"validation_code"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// FieldValidations groups validation results by field name
type FieldValidations struct {
	FieldName   string              `json:"field_name"`
	Validations []*ValidationResult `json:"validations"`
}

// HasValidations returns true if there are any validation results for this field
func (f *FieldValidations) HasValidations() bool {
	return len(f.Validations) > 0
}

// HasErrors returns true if any validation result for this field is invalid
func (f *FieldValidations) HasErrors() bool {
	for _, validation := range f.Validations {
		if !validation.IsValid {
			return true
		}
	}
	return false
}

// FieldValidationResults is a collection of field validations
type FieldValidationResults []*FieldValidations

// GetFieldValidations returns the validations for a specific field, creating a new entry if not found
func (f FieldValidationResults) GetFieldValidations(fieldName string) *FieldValidations {
	for _, fieldValidation := range f {
		if fieldValidation.FieldName == fieldName {
			return fieldValidation
		}
	}
	return &FieldValidations{FieldName: fieldName, Validations: []*ValidationResult{}}
}

// HasErrors returns true if any field has validation errors
func (f FieldValidationResults) HasErrors() bool {
	for _, fieldValidation := range f {
		if fieldValidation.HasErrors() {
			return true
		}
	}
	return false
}

// Validation options

// WithValue sets a custom value for display
func WithValue(value string) ValidationOption {
	return func(vr *ValidationResult) {
		vr.Value = value
	}
}

// WithMessage sets a custom validation message
func WithMessage(message string) ValidationOption {
	return func(vr *ValidationResult) {
		vr.Message = message
	}
}

// WithSuggestedAction sets a custom suggested action
func WithSuggestedAction(action string) ValidationOption {
	return func(vr *ValidationResult) {
		vr.SuggestedAction = action
	}
}

func WithMaskedValue(value string) ValidationOption {
	return func(vr *ValidationResult) {
		vr.Value = MaskString(value)
	}
}

// WithValidationCode sets the validation code
func WithValidationCode(code ValidationCode) ValidationOption {
	return func(vr *ValidationResult) {
		vr.ValidationCode = code
	}
}

// WithMetadata adds metadata to the validation result
func WithMetadata(key string, value interface{}) ValidationOption {
	return func(vr *ValidationResult) {
		if vr.Metadata == nil {
			vr.Metadata = make(map[string]interface{})
		}
		vr.Metadata[key] = value
	}
}

// WithMetadataMap adds multiple metadata entries
func WithMetadataMap(metadata map[string]interface{}) ValidationOption {
	return func(vr *ValidationResult) {
		if vr.Metadata == nil {
			vr.Metadata = make(map[string]interface{})
		}
		for k, v := range metadata {
			vr.Metadata[k] = v
		}
	}
}

// NewValidationResult creates a new ValidationResult
func NewValidationResult(isValid bool, fieldName string, options ...ValidationOption) *ValidationResult {
	vr := &ValidationResult{
		IsValid:         isValid,
		FieldName:       fieldName,
		Value:           "",
		Message:         "",
		SuggestedAction: "",
		ValidationCode:  ValidationCodeUnspecified,
		Metadata:        make(map[string]interface{}),
	}

	// Apply options
	for _, option := range options {
		option(vr)
	}

	return vr
}

// GetMetadata returns a metadata value by key
func (vr *ValidationResult) GetMetadata(key string) (interface{}, bool) {
	if vr.Metadata == nil {
		return nil, false
	}
	value, exists := vr.Metadata[key]
	return value, exists
}

// SetMetadata sets a metadata value by key
func (vr *ValidationResult) SetMetadata(key string, value interface{}) {
	if vr.Metadata == nil {
		vr.Metadata = make(map[string]interface{})
	}
	vr.Metadata[key] = value
}

func (vr *ValidationResult) ToAppError() *eventsourcing.AppError {
	// If the validation result is valid, return nil
	if vr.IsValid {
		return nil
	}

	// Create a new app error
	appError := &eventsourcing.AppError{
		Code:     string(vr.ValidationCode),
		Message:  vr.Message,
		Solution: vr.SuggestedAction,
	}
	for key, value := range vr.Metadata {
		appError.Details[key] = fmt.Sprintf("%v", value)
	}
	appError.Details["field_name"] = vr.FieldName
	appError.Details["value"] = vr.Value
	return appError
}

// ValidationBuilder helps build collections of validation results
type ValidationBuilder struct {
	results map[string][]*ValidationResult
}

// NewValidationBuilder creates a new validation builder
func NewValidationBuilder() *ValidationBuilder {
	return &ValidationBuilder{
		results: make(map[string][]*ValidationResult),
	}
}

// Add adds a validation result to the builder with additional options applied
func (b *ValidationBuilder) Add(result *ValidationResult, options ...ValidationOption) *ValidationBuilder {
	// Apply options to the result
	for _, option := range options {
		option(result)
	}
	b.results[result.FieldName] = append(b.results[result.FieldName], result)
	return b
}

// Build returns all validation results grouped by field
func (b *ValidationBuilder) Build() FieldValidationResults {
	fieldValidations := make(FieldValidationResults, 0, len(b.results))
	for fieldName, results := range b.results {
		fieldValidations = append(fieldValidations, &FieldValidations{
			FieldName:   fieldName,
			Validations: results,
		})
	}
	return fieldValidations
}

// BuildErrors returns only validation results that have errors
func (b *ValidationBuilder) BuildErrors() FieldValidationResults {
	fieldValidations := make(FieldValidationResults, 0)
	for fieldName, results := range b.results {
		var errorResults []*ValidationResult
		for _, result := range results {
			if !result.IsValid {
				errorResults = append(errorResults, result)
			}
		}
		if len(errorResults) > 0 {
			fieldValidations = append(fieldValidations, &FieldValidations{
				FieldName:   fieldName,
				Validations: errorResults,
			})
		}
	}
	return fieldValidations
}
