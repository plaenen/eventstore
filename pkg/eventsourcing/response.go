package eventsourcing

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// NewSuccessResponse creates a successful Response with data
func NewSuccessResponse(data proto.Message) (*Response, error) {
	anyData, err := anypb.New(data)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data: %w", err)
	}

	return &Response{
		Success: true,
		Data:    anyData,
	}, nil
}

// NewErrorResponse creates an error Response with AppError
func NewErrorResponse(code, message, solution string, details map[string]string) *Response {
	return &Response{
		Success: false,
		Error: &AppError{
			Code:     code,
			Message:  message,
			Solution: solution,
			Details:  details,
		},
	}
}

// NewSimpleErrorResponse creates an error Response with just code and message
func NewSimpleErrorResponse(code, message string) *Response {
	return NewErrorResponse(code, message, "", nil)
}

// UnpackData unpacks the response data into the target message
func (r *Response) UnpackData(target proto.Message) error {
	if !r.Success {
		return fmt.Errorf("cannot unpack data from failed response: %s", r.Error.Message)
	}

	if r.Data == nil {
		return fmt.Errorf("response has no data")
	}

	return r.Data.UnmarshalTo(target)
}

// AsError converts a failed Response to a Go error
// Returns nil if the response was successful
func (r *Response) AsError() error {
	if r.Success {
		return nil
	}

	if r.GetError() == nil {
		return fmt.Errorf("operation failed")
	}

	return &ResponseError{AppError: r.GetError()}
}

// ResponseError wraps an AppError as a Go error
type ResponseError struct {
	AppError *AppError
}

func (e *ResponseError) Error() string {
	if e.AppError.Solution != "" {
		return fmt.Sprintf("%s (code: %s). Solution: %s",
			e.AppError.Message, e.AppError.Code, e.AppError.Solution)
	}
	return fmt.Sprintf("%s (code: %s)", e.AppError.Message, e.AppError.Code)
}

// Code returns the error code
func (e *ResponseError) Code() string {
	return e.AppError.Code
}

// Solution returns the suggested solution
func (e *ResponseError) Solution() string {
	return e.AppError.Solution
}

// Details returns additional error details
func (e *ResponseError) Details() map[string]string {
	return e.AppError.Details
}
