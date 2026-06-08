package errors

import "net/http"

// ToHTTP converts an error into an HTTP status code and a JSON-serializable body.
//
// The returned body matches the project's Status shape:
// { code, reason, message, metadata }.
func ToHTTP(err error) (statusCode int, body Status) {
	if err == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}

	appErr := FromError(err)
	if appErr == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}

	body = Status{
		Code:    appErr.Code,
		Reason:  appErr.Reason,
		Message: appErr.Message,
	}
	if appErr.Metadata != nil {
		body.Metadata = make(map[string]string, len(appErr.Metadata))
		for k, v := range appErr.Metadata {
			body.Metadata[k] = v
		}
	}
	return int(appErr.Code), body
}
