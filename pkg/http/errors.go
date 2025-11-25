package http

import (
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"

	"github.com/plaenen/eventstore/pkg/errorx"
	"github.com/plaenen/eventstore/pkg/protocol"
)

// ErrorResponse is the JSON structure sent to HTTP clients.
//
// This provides a consistent error format across all HTTP endpoints:
//
//	{
//	  "code": "NOT_FOUND",
//	  "message": "Aggregate not found",
//	  "detail": "No aggregate exists with ID '123'",
//	  "hint": "Verify the aggregate ID is correct"
//	}
type ErrorResponse struct {
	Code    string `json:"code"`             // Machine-readable code
	Message string `json:"message"`          // Human-readable summary
	Detail  string `json:"detail,omitempty"` // Specific details about this error
	Hint    string `json:"hint,omitempty"`   // Actionable suggestion
}

// Handler wraps an HTTP handler function that can return an error.
//
// This allows you to write handlers that return errors directly:
//
//	func (h *Handler) GetAggregate(w http.ResponseWriter, r *http.Request) error {
//	    id := r.PathValue("id")
//	    agg, err := h.repo.Load(id)
//	    if err != nil {
//	        return err  // Automatically converted to HTTP response
//	    }
//	    return json.NewEncoder(w).Encode(agg)
//	}
//
//	// Register with automatic error handling
//	mux.Handle("GET /aggregates/{id}", http.Handler(handler.GetAggregate))
type Handler func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler interface
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		HandleError(w, r, err)
	}
}

// HandleError converts domain/application errors to HTTP JSON responses.
//
// Error Translation:
//   - errorx.ErrNotFound → 404 Not Found
//   - errorx.ErrAlreadyExists → 409 Conflict
//   - errorx.ErrConflict → 409 Conflict (version conflicts)
//   - errorx.ErrInvalidArgument → 400 Bad Request
//   - errorx.ErrPermissionDenied → 403 Forbidden
//   - errorx.ErrUnauthenticated → 401 Unauthorized
//   - errorx.ErrTimeout → 503 Service Unavailable
//   - protocol.AppError → Uses AppError.HTTPStatus
//   - Other errors → 500 Internal Server Error (sanitized)
//
// Usage:
//
//	if err := handler(w, r); err != nil {
//	    http.HandleError(w, r, err)
//	}
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	var resp ErrorResponse
	var status int

	// First check if it's a protocol.AppError (wire protocol)
	var appErr *protocol.AppError
	if AsAppError(err, &appErr) {
		status = mapCodeToHTTPStatus(appErr.Code)
		resp = ErrorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
			Detail:  getDetail(appErr.Details),
			Hint:    appErr.Solution,
		}

		// Log internal context if present
		if len(appErr.Details) > 0 {
			slog.Error("request failed",
				"method", r.Method,
				"path", r.URL.Path,
				"code", appErr.Code,
				"details", appErr.Details,
			)
		}
		writeJSON(w, status, resp)
		return
	}

	// Check for structured error types from pkg/errors
	var notFoundErr *errorx.NotFoundError
	var conflictErr *errorx.ConflictError
	var uniqueErr *errorx.UniqueConstraintError
	var validationErr *errorx.ValidationError

	switch {
	case stderrors.As(err, &notFoundErr):
		status = http.StatusNotFound
		resp = ErrorResponse{
			Code:    "NOT_FOUND",
			Message: notFoundErr.Error(),
			Detail:  notFoundErr.Error(),
			Hint:    "Verify the ID is correct, or check if the resource was deleted",
		}

	case stderrors.As(err, &conflictErr):
		status = http.StatusConflict
		resp = ErrorResponse{
			Code:    "VERSION_CONFLICT",
			Message: "Version conflict detected",
			Detail:  conflictErr.Error(),
			Hint:    "Reload the resource and retry your operation",
		}

	case stderrors.As(err, &uniqueErr):
		status = http.StatusConflict
		resp = ErrorResponse{
			Code:    "ALREADY_EXISTS",
			Message: "Resource already exists",
			Detail:  uniqueErr.Error(),
			Hint:    "Use a different value or update the existing resource",
		}

	case stderrors.As(err, &validationErr):
		status = http.StatusBadRequest
		resp = ErrorResponse{
			Code:    "VALIDATION_ERROR",
			Message: "Validation failed",
			Detail:  validationErr.Error(),
			Hint:    validationErr.Message,
		}

	// Check for sentinel errors from pkg/errors
	case stderrors.Is(err, errorx.ErrNotFound):
		status = http.StatusNotFound
		resp = ErrorResponse{
			Code:    "NOT_FOUND",
			Message: "Resource not found",
			Detail:  err.Error(),
			Hint:    "Verify the resource ID is correct",
		}

	case stderrors.Is(err, errorx.ErrAlreadyExists):
		status = http.StatusConflict
		resp = ErrorResponse{
			Code:    "ALREADY_EXISTS",
			Message: "Resource already exists",
			Detail:  err.Error(),
			Hint:    "Use a different identifier or update the existing resource",
		}

	case stderrors.Is(err, errorx.ErrConflict) || stderrors.Is(err, errorx.ErrConcurrencyConflict):
		status = http.StatusConflict
		resp = ErrorResponse{
			Code:    "VERSION_CONFLICT",
			Message: "Concurrent modification detected",
			Detail:  err.Error(),
			Hint:    "Reload the resource and retry your operation",
		}

	case stderrors.Is(err, errorx.ErrInvalidArgument):
		status = http.StatusBadRequest
		resp = ErrorResponse{
			Code:    "INVALID_ARGUMENT",
			Message: "Invalid request",
			Detail:  err.Error(),
			Hint:    "Check your request parameters and try again",
		}

	case stderrors.Is(err, errorx.ErrPermissionDenied):
		status = http.StatusForbidden
		resp = ErrorResponse{
			Code:    "PERMISSION_DENIED",
			Message: "Permission denied",
			Detail:  "You don't have permission to perform this action",
			Hint:    "Contact an administrator if you believe you should have access",
		}

	case stderrors.Is(err, errorx.ErrUnauthenticated):
		status = http.StatusUnauthorized
		resp = ErrorResponse{
			Code:    "UNAUTHENTICATED",
			Message: "Authentication required",
			Detail:  "You must be authenticated to access this resource",
			Hint:    "Provide valid authentication credentials",
		}

	case stderrors.Is(err, errorx.ErrPreconditionFailed):
		status = http.StatusPreconditionFailed
		resp = ErrorResponse{
			Code:    "PRECONDITION_FAILED",
			Message: "Precondition not met",
			Detail:  err.Error(),
			Hint:    "Ensure all required conditions are satisfied before retrying",
		}

	case stderrors.Is(err, errorx.ErrResourceExhausted):
		status = http.StatusTooManyRequests
		resp = ErrorResponse{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: "Rate limit exceeded",
			Detail:  err.Error(),
			Hint:    "Wait a moment before making additional requests",
		}

	case stderrors.Is(err, errorx.ErrTimeout) || stderrors.Is(err, errorx.ErrUnavailable):
		status = http.StatusServiceUnavailable
		resp = ErrorResponse{
			Code:    "SERVICE_UNAVAILABLE",
			Message: "Service temporarily unavailable",
			Detail:  "The service is temporarily unable to handle your request",
			Hint:    "Please retry your request in a few moments",
		}

	case stderrors.Is(err, errorx.ErrDataCorruption):
		status = http.StatusInternalServerError
		resp = ErrorResponse{
			Code:    "DATA_CORRUPTION",
			Message: "Data integrity issue detected",
			Detail:  "The requested data appears to be corrupted",
			Hint:    "Contact support for assistance",
		}

	default:
		// Unknown error - don't expose internal details (security)
		status = http.StatusInternalServerError
		resp = ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "An unexpected error occurred",
			Detail:  "We encountered an issue processing your request",
			Hint:    "Please try again later. Contact support if the problem persists",
		}

		// Log the actual error for debugging
		slog.Error("unexpected HTTP error",
			"method", r.Method,
			"path", r.URL.Path,
			"error", err,
		)
	}

	writeJSON(w, status, resp)
}

// writeJSON writes a JSON response with the given status code
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// mapCodeToHTTPStatus maps protocol error codes to HTTP status codes
func mapCodeToHTTPStatus(code string) int {
	switch code {
	case protocol.ErrCodeInvalidArgument:
		return http.StatusBadRequest
	case protocol.ErrCodeNotFound:
		return http.StatusNotFound
	case protocol.ErrCodeAlreadyExists:
		return http.StatusConflict
	case protocol.ErrCodePermissionDenied:
		return http.StatusForbidden
	case protocol.ErrCodeUnauthenticated:
		return http.StatusUnauthorized
	case protocol.ErrCodeResourceExhausted:
		return http.StatusTooManyRequests
	case protocol.ErrCodeTimeout:
		return http.StatusGatewayTimeout
	case protocol.ErrCodeConflict:
		return http.StatusConflict
	case protocol.ErrCodeInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// getDetail extracts a single detail string from AppError details map
func getDetail(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}
	// Return first detail value
	for _, v := range details {
		return v
	}
	return ""
}

// AsAppError is a type assertion helper for protocol.AppError
func AsAppError(err error, target **protocol.AppError) bool {
	return stderrors.As(err, target)
}
