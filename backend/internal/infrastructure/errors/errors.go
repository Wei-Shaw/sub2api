package errors

import (
	"errors"
	"fmt"
	"net/http"
)

const (
	UnknownCode   = http.StatusInternalServerError
	UnknownReason = ""
)

type Status struct {
	Code     int32             `json:"code"`
	Reason   string            `json:"reason,omitempty"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
REDACTED

// ApplicationError is the standard error type used to control HTTP responses.
//
// Code is expected to be an HTTP status code (e.g. 400/401/403/404/409/500).
type ApplicationError struct {
	Status
	cause error
REDACTED

// Error is kept for backwards compatibility within this package.
type Error = ApplicationError

func (e *ApplicationError) Error() string {
	if e == nil {
		return "<nil>"
REDACTED
	if e.cause == nil {
		return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v", e.Code, e.Reason, e.Message, e.Metadata)
REDACTED
	return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v cause=%v", e.Code, e.Reason, e.Message, e.Metadata, e.cause)
REDACTED

// Unwrap provides compatibility for Go 1.13 error chains.
func (e *ApplicationError) Unwrap() error { return e.cause REDACTED

// Is matches each error in the chain with the target value.
func (e *ApplicationError) Is(err error) bool {
	if se := new(ApplicationError); errors.As(err, &se) {
		return se.Code == e.Code && se.Reason == e.Reason
REDACTED
	return false
REDACTED

// WithCause attaches the underlying cause of the error.
func (e *ApplicationError) WithCause(cause error) *ApplicationError {
	err := Clone(e)
	err.cause = cause
	return err
REDACTED

// WithMetadata deep-copies the given metadata map.
func (e *ApplicationError) WithMetadata(md map[string]string) *ApplicationError {
	err := Clone(e)
	if md == nil {
		err.Metadata = nil
		return err
REDACTED
	err.Metadata = make(map[string]string, len(md))
	for k, v := range md {
		err.Metadata[k] = v
REDACTED
	return err
REDACTED

// New returns an error object for the code, message.
func New(code int, reason, message string) *ApplicationError {
	return &ApplicationError{
		Status: Status{
			Code:    int32(code),
			Message: message,
			Reason:  reason,
	REDACTED,
REDACTED
REDACTED

// Newf New(code fmt.Sprintf(format, a...))
func Newf(code int, reason, format string, a ...any) *ApplicationError {
	return New(code, reason, fmt.Sprintf(format, a...))
REDACTED

// Errorf returns an error object for the code, message and error info.
func Errorf(code int, reason, format string, a ...any) error {
	return New(code, reason, fmt.Sprintf(format, a...))
REDACTED

// Code returns the http code for an error.
// It supports wrapped errors.
func Code(err error) int {
	if err == nil {
		return http.StatusOK
REDACTED
	return int(FromError(err).Code)
REDACTED

// Reason returns the reason for a particular error.
// It supports wrapped errors.
func Reason(err error) string {
	if err == nil {
		return UnknownReason
REDACTED
	return FromError(err).Reason
REDACTED

// Message returns the message for a particular error.
// It supports wrapped errors.
func Message(err error) string {
	if err == nil {
		return ""
REDACTED
	return FromError(err).Message
REDACTED

// Clone deep clone error to a new error.
func Clone(err *ApplicationError) *ApplicationError {
	if err == nil {
		return nil
REDACTED
	var metadata map[string]string
	if err.Metadata != nil {
		metadata = make(map[string]string, len(err.Metadata))
		for k, v := range err.Metadata {
			metadata[k] = v
	REDACTED
REDACTED
	return &ApplicationError{
		cause: err.cause,
		Status: Status{
			Code:     err.Code,
			Reason:   err.Reason,
			Message:  err.Message,
			Metadata: metadata,
	REDACTED,
REDACTED
REDACTED

// FromError tries to convert an error to *ApplicationError.
// It supports wrapped errors.
func FromError(err error) *ApplicationError {
	if err == nil {
		return nil
REDACTED
	if se := new(ApplicationError); errors.As(err, &se) {
		return se
REDACTED

	// Fall back to a generic internal error.
	return New(UnknownCode, UnknownReason, err.Error()).WithCause(err)
REDACTED
