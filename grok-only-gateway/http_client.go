package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPClientFactory struct {
	timeout time.Duration
}

func (f *HTTPClientFactory) Client(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	timeout := f.timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 180 * time.Second
	}
	return context.WithTimeout(ctx, d)
}
