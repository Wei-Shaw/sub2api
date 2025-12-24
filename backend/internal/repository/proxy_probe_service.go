package repository

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"sub2api/internal/config"
	"sub2api/internal/service"

	"golang.org/x/net/proxy"
)

type proxyProbeService struct {
	insecureSkipVerify bool
}

func NewProxyExitInfoProber(cfg *config.Config) service.ProxyExitInfoProber {
	// 默认启用 TLS 校验，只有显式配置才会跳过验证。
	return &proxyProbeService{insecureSkipVerify: cfg.Proxy.TLSInsecureSkipVerify}
}

func (s *proxyProbeService) ProbeProxy(ctx context.Context, proxyURL string) (*service.ProxyExitInfo, int64, error) {
	transport, err := createProxyTransport(proxyURL, s.insecureSkipVerify)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create proxy transport: %w", err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	startTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipinfo.io/json", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	latencyMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, latencyMs, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	var ipInfo struct {
		IP      string `json:"ip"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latencyMs, fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, &ipInfo); err != nil {
		return nil, latencyMs, fmt.Errorf("failed to parse response: %w", err)
	}

	return &service.ProxyExitInfo{
		IP:      ipInfo.IP,
		City:    ipInfo.City,
		Region:  ipInfo.Region,
		Country: ipInfo.Country,
	}, latencyMs, nil
}

func createProxyTransport(proxyURL string, insecureSkipVerify bool) (*http.Transport, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
	}

	switch parsedURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedURL)
	case "socks5":
		dialer, err := proxy.FromURL(parsedURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create socks5 dialer: %w", err)
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return nil, fmt.Errorf("unsupported proxy protocol: %s", parsedURL.Scheme)
	}

	return transport, nil
}
