package googleapi

import (
	"net/http"
	"testing"
)

func TestHTTPStatusToGoogleStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "INVALID_ARGUMENT"},
		{http.StatusUnauthorized, "UNAUTHENTICATED"},
		{http.StatusForbidden, "PERMISSION_DENIED"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusTooManyRequests, "RESOURCE_EXHAUSTED"},
		{499, "CANCELLED"},
		{http.StatusBadGateway, "INTERNAL"},
		{http.StatusServiceUnavailable, "INTERNAL"},
		{http.StatusConflict, "UNKNOWN"},
	}
	for _, tc := range cases {
		if got := HTTPStatusToGoogleStatus(tc.status); got != tc.want {
			t.Errorf("HTTPStatusToGoogleStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
