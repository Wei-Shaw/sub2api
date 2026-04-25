// Package errors provides a minimal copy of the core's ApplicationError
// abstraction so the plugin's services and handlers can raise structured
// errors without importing the core's internal package.
//
// The shape mirrors backend/internal/pkg/errors so HTTP responses produced by
// the plugin look identical to the rest of the API.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

const (
	UnknownCode    = http.StatusInternalServerError
	UnknownReason  = ""
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
		return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v", e.Code, e.Reason, e.Message, e.Metadata)
	}
	return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v cause=%v", e.Code, e.Reason, e.Message, e.Metadata, e.cause)
}

func (e *ApplicationError) Unwrap() error { return e.cause }

func (e *ApplicationError) Is(err error) bool {
	if se := new(ApplicationError); errors.As(err, &se) {
		return se.Code == e.Code && se.Reason == e.Reason
	}
	return false
}

func (e *ApplicationError) WithCause(cause error) *ApplicationError {
	out := Clone(e)
	out.cause = cause
	return out
}

func (e *ApplicationError) WithMetadata(md map[string]string) *ApplicationError {
	out := Clone(e)
	if md == nil {
		out.Metadata = nil
		return out
	}
	out.Metadata = make(map[string]string, len(md))
	for k, v := range md {
		out.Metadata[k] = v
	}
	return out
}

func New(code int, reason, message string) *ApplicationError {
	return &ApplicationError{Status: Status{Code: int32(code), Message: message, Reason: reason}}
}

func Clone(err *ApplicationError) *ApplicationError {
	if err == nil {
		return nil
	}
	var metadata map[string]string
	if err.Metadata != nil {
		metadata = make(map[string]string, len(err.Metadata))
		for k, v := range err.Metadata {
			metadata[k] = v
		}
	}
	return &ApplicationError{
		cause:  err.cause,
		Status: Status{Code: err.Code, Reason: err.Reason, Message: err.Message, Metadata: metadata},
	}
}

func FromError(err error) *ApplicationError {
	if err == nil {
		return nil
	}
	if se := new(ApplicationError); errors.As(err, &se) {
		return se
	}
	return New(UnknownCode, UnknownReason, UnknownMessage).WithCause(err)
}

// BadRequest returns a 400 ApplicationError.
func BadRequest(reason, message string) *ApplicationError {
	return New(http.StatusBadRequest, reason, message)
}

// NotFound returns a 404 ApplicationError.
func NotFound(reason, message string) *ApplicationError {
	return New(http.StatusNotFound, reason, message)
}

// Conflict returns a 409 ApplicationError.
func Conflict(reason, message string) *ApplicationError {
	return New(http.StatusConflict, reason, message)
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
	if appErr == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}
	body := Status{Code: appErr.Code, Reason: appErr.Reason, Message: appErr.Message}
	if appErr.Metadata != nil {
		body.Metadata = make(map[string]string, len(appErr.Metadata))
		for k, v := range appErr.Metadata {
			body.Metadata[k] = v
		}
	}
	return int(appErr.Code), body
}
