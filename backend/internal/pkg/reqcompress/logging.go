package reqcompress

import (
	"net/http"
	"strings"
)

// logHost returns the upstream host only. Paths and query strings are omitted
// because some providers carry credentials in the URL.
func logHost(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.Host
}

// upstreamRequestID picks common request-id response headers for log
// correlation. Missing headers yield an empty string.
func upstreamRequestID(resp *http.Response) string {
	if resp == nil || resp.Header == nil {
		return ""
	}
	for _, key := range []string{"x-request-id", "request-id", "x-amzn-requestid", "cf-ray"} {
		if value := strings.TrimSpace(resp.Header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}
