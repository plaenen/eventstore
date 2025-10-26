package validators

import "fmt"

type ValidationError struct {
	Code            string
	Message         string
	SuggestedAction string
	Details         map[string]string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("Validation error: code=%s, message=%s, suggested_action=%s, details=%v", e.Code, e.Message, e.SuggestedAction, e.Details)
}
