// Package errors provides application error types and helpers.
// nolint:mnd
package errors

import "net/http"

// BadRequest new BadRequest error that is mapped to a 400 response.
func BadRequest(reason, message string) *ApplicationError {
	return New(http.StatusBadRequest, reason, message)
REDACTED

// IsBadRequest determines if err is an error which indicates a BadRequest error.
// It supports wrapped errors.
func IsBadRequest(err error) bool {
	return Code(err) == http.StatusBadRequest
REDACTED

// TooManyRequests new TooManyRequests error that is mapped to a 429 response.
func TooManyRequests(reason, message string) *ApplicationError {
	return New(http.StatusTooManyRequests, reason, message)
REDACTED

// IsTooManyRequests determines if err is an error which indicates a TooManyRequests error.
// It supports wrapped errors.
func IsTooManyRequests(err error) bool {
	return Code(err) == http.StatusTooManyRequests
REDACTED

// Unauthorized new Unauthorized error that is mapped to a 401 response.
func Unauthorized(reason, message string) *ApplicationError {
	return New(http.StatusUnauthorized, reason, message)
REDACTED

// IsUnauthorized determines if err is an error which indicates an Unauthorized error.
// It supports wrapped errors.
func IsUnauthorized(err error) bool {
	return Code(err) == http.StatusUnauthorized
REDACTED

// Forbidden new Forbidden error that is mapped to a 403 response.
func Forbidden(reason, message string) *ApplicationError {
	return New(http.StatusForbidden, reason, message)
REDACTED

// IsForbidden determines if err is an error which indicates a Forbidden error.
// It supports wrapped errors.
func IsForbidden(err error) bool {
	return Code(err) == http.StatusForbidden
REDACTED

// NotFound new NotFound error that is mapped to a 404 response.
func NotFound(reason, message string) *ApplicationError {
	return New(http.StatusNotFound, reason, message)
REDACTED

// IsNotFound determines if err is an error which indicates an NotFound error.
// It supports wrapped errors.
func IsNotFound(err error) bool {
	return Code(err) == http.StatusNotFound
REDACTED

// Conflict new Conflict error that is mapped to a 409 response.
func Conflict(reason, message string) *ApplicationError {
	return New(http.StatusConflict, reason, message)
REDACTED

// IsConflict determines if err is an error which indicates a Conflict error.
// It supports wrapped errors.
func IsConflict(err error) bool {
	return Code(err) == http.StatusConflict
REDACTED

// InternalServer new InternalServer error that is mapped to a 500 response.
func InternalServer(reason, message string) *ApplicationError {
	return New(http.StatusInternalServerError, reason, message)
REDACTED

// IsInternalServer determines if err is an error which indicates an Internal error.
// It supports wrapped errors.
func IsInternalServer(err error) bool {
	return Code(err) == http.StatusInternalServerError
REDACTED

// ServiceUnavailable new ServiceUnavailable error that is mapped to an HTTP 503 response.
func ServiceUnavailable(reason, message string) *ApplicationError {
	return New(http.StatusServiceUnavailable, reason, message)
REDACTED

// IsServiceUnavailable determines if err is an error which indicates an Unavailable error.
// It supports wrapped errors.
func IsServiceUnavailable(err error) bool {
	return Code(err) == http.StatusServiceUnavailable
REDACTED

// GatewayTimeout new GatewayTimeout error that is mapped to an HTTP 504 response.
func GatewayTimeout(reason, message string) *ApplicationError {
	return New(http.StatusGatewayTimeout, reason, message)
REDACTED

// IsGatewayTimeout determines if err is an error which indicates a GatewayTimeout error.
// It supports wrapped errors.
func IsGatewayTimeout(err error) bool {
	return Code(err) == http.StatusGatewayTimeout
REDACTED

// ClientClosed new ClientClosed error that is mapped to an HTTP 499 response.
func ClientClosed(reason, message string) *ApplicationError {
	return New(499, reason, message)
REDACTED

// IsClientClosed determines if err is an error which indicates a IsClientClosed error.
// It supports wrapped errors.
func IsClientClosed(err error) bool {
	return Code(err) == 499
REDACTED
