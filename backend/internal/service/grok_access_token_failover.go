package service

import "net/http"

func newGrokAccessTokenFailoverError(err error) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":"grok access token unavailable; switching account"}`),
	}
}
