package errors

import "net/http"

// ToHTTP converts an error into an HTTP status code and a JSON-serializable body.
//
// The returned body matches the project's Status shape:
// { code, reason, message, metadata REDACTED.
func ToHTTP(err error) (statusCode int, body Status) {
	if err == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)REDACTED
REDACTED

	appErr := FromError(err)
	if appErr == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)REDACTED
REDACTED

	cloned := Clone(appErr)
	return int(cloned.Code), cloned.Status
REDACTED
