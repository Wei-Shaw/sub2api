package backend

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
)

func (adapter *OpenAIAdapter) Handle(ctx context.Context, request PlatformRequest) (PlatformResponse, error) {
	return forwardPlatformRequest(ctx, request, request.Route, request.Endpoint, request.Method, request.Body)
}

func forwardPlatformRequest(ctx context.Context, request PlatformRequest, config RouteConfig, endpoint, method string, body []byte) (PlatformResponse, error) {
	upstreamURL, err := ResolveRouteEndpointURL(config, endpoint)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	parsedURL, err := url.Parse(upstreamURL)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	parsedURL.RawQuery = request.Query
	upstreamRequest, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewReader(body))
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	copyEndToEndHeaders(upstreamRequest.Header, request.Headers)
	response, err := request.Client.Do(upstreamRequest)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	return PlatformResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: responseBody}, nil
}
