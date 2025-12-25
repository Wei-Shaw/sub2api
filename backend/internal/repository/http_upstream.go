package repository

import (
	"net/http"
	"net/url"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/ports"
)

// httpUpstreamService is a generic HTTP upstream service that can be used for
// making requests to any HTTP API (Claude, OpenAI, etc.) with optional proxy support.
type httpUpstreamService struct {
	defaultClient *http.Client
	cfg           *config.Config
}

// NewHTTPUpstream creates a new generic HTTP upstream service
func NewHTTPUpstream(cfg *config.Config) ports.HTTPUpstream {
	responseHeaderTimeout := time.Duration(cfg.Gateway.ResponseHeaderTimeout) * time.Second
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 300 * time.Second
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}

	return &httpUpstreamService{
		defaultClient: &http.Client{Transport: transport},
		cfg:           cfg,
	}
}

func (s *httpUpstreamService) Do(req *http.Request, proxyURL string) (*http.Response, error) {
	if proxyURL == "" {
		return s.defaultClient.Do(req)
	}
	client := s.createProxyClient(proxyURL)
	return client.Do(req)
}

func (s *httpUpstreamService) createProxyClient(proxyURL string) *http.Client {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return s.defaultClient
	}

	responseHeaderTimeout := time.Duration(s.cfg.Gateway.ResponseHeaderTimeout) * time.Second
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 300 * time.Second
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(parsedURL),
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}

	return &http.Client{Transport: transport}
}
