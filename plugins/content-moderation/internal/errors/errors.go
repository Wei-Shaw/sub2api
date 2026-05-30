// Package errors provides a minimal copy of the core's ApplicationError
// abstraction so the plugin's services and handlers can raise structured
// errors without importing the core's internal package. The shape mirrors
// backend/internal/pkg/errors so HTTP responses produced by the plugin look
// identical to the rest of the API.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

const (
	UnknownCode    = http.StatusInternalServerError
	UnknownReason  = "INTERNAL_ERROR"
	UnknownMessage = "internal error"
)

// Status is the JSON-serialisable shape of an error.
type Status struct {
	Code     int32             `json:"code"`
	Reason   string            `json:"reason,omitempty"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ApplicationError carries an HTTP status code, a stable reason key, a
// human-readable message and optional metadata.
type ApplicationError struct {
	Status
	cause error
}

func (e *ApplicationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.cause == nil {
		return fmt.Sprintf("error: code=%d reason=%q message=%q", e.Code, e.Reason, e.Message)
	}
	return fmt.Sprintf("error: code=%d reason=%q message=%q cause=%v", e.Code, e.Reason, e.Message, e.cause)
}

func (e *ApplicationError) Unwrap() error { return e.cause }

func (e *ApplicationError) Is(err error) bool {
	if se := new(ApplicationError); errors.As(err, &se) {
		return se.Code == e.Code && se.Reason == e.Reason
	}
	return false
}

func (e *ApplicationError) WithCause(cause error) *ApplicationError {
	out := *e
	out.cause = cause
	return &out
}

func New(code int, reason, message string) *ApplicationError {
	return &ApplicationError{Status: Status{Code: int32(code), Message: message, Reason: reason}}
}

func FromError(err error) *ApplicationError {
	if err == nil {
		return nil
	}
	if se := new(ApplicationError); errors.As(err, &se) {
		return se
	}
	return New(UnknownCode, UnknownReason, err.Error()).WithCause(err)
}

// BadRequest returns a 400 ApplicationError.
func BadRequest(reason, message string) *ApplicationError {
	return New(http.StatusBadRequest, reason, message)
}

// NotFound returns a 404 ApplicationError.
func NotFound(reason, message string) *ApplicationError {
	return New(http.StatusNotFound, reason, message)
}

// InternalServer returns a 500 ApplicationError.
func InternalServer(reason, message string) *ApplicationError {
	return New(http.StatusInternalServerError, reason, message)
}

// ToHTTP turns any error into a status code + JSON body.
func ToHTTP(err error) (int, Status) {
	if err == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}
	appErr := FromError(err)
	body := Status{Code: appErr.Code, Reason: appErr.Reason, Message: appErr.Message, Metadata: appErr.Metadata}
	return int(appErr.Code), body
}
