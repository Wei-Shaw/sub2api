package securityaudit

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const maxGuardResponseBytes int64 = 256 * 1024

var lookupPromptAuditIP = net.DefaultResolver.LookupIPAddr

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url_scheme", "审计节点仅支持 HTTP(S)")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", infraerrors.BadRequest("prompt_audit_unsafe_base_url", "审计节点地址不能包含凭据、查询参数或片段")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.EqualFold(path, "/v1") {
		path = ""
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ChatCompletionsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/chat/completions", nil
}

func ModelsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/models", nil
}

func NewSecureHTTPClient(endpoint ActiveEndpoint) (*http.Client, error) {
	normalized, err := NormalizeBaseURL(endpoint.BaseURL)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(normalized)
	if endpoint.EngineType == EngineGenericLLM {
		if literal := net.ParseIP(parsed.Hostname()); literal != nil && !isPublicPromptAuditIP(literal) {
			return nil, infraerrors.BadRequest("prompt_audit_unsafe_destination", "通用审计节点必须使用公网地址")
		}
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		// Do not inherit HTTP(S)_PROXY. A proxy would move the actual destination
		// dial outside secureDialContext and bypass this module's DNS/IP validation.
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: time.Duration(endpoint.TimeoutMS) * time.Millisecond,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = dialer.DialContext
	if endpoint.EngineType == EngineGenericLLM {
		transport.DialContext = secureGenericDialContext(dialer.DialContext, parsed.Hostname())
		if parsed.Scheme == "https" {
			transport.TLSClientConfig.ServerName = parsed.Hostname()
		}
	}
	timeout := time.Duration(endpoint.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeoutMS * time.Millisecond
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	if endpoint.EngineType == EngineGenericLLM {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return infraerrors.BadRequest("prompt_audit_redirect_rejected", "通用审计节点不能重定向")
		}
	}
	return client, nil
}

func secureGenericDialContext(dial func(context.Context, string, string) (net.Conn, error), configuredHost string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(configuredHost, ".")) {
			return nil, infraerrors.BadRequest("prompt_audit_destination_mismatch", "通用审计节点目标无效")
		}
		addresses, err := lookupPromptAuditIP(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve prompt audit endpoint: %w", err)
		}
		if len(addresses) == 0 {
			return nil, errors.New("resolve prompt audit endpoint: no addresses")
		}
		for _, address := range addresses {
			if !isPublicPromptAuditIP(address.IP) {
				return nil, infraerrors.BadRequest("prompt_audit_unsafe_destination", "通用审计节点必须解析到公网地址")
			}
		}
		return dial(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}
}

func isPublicPromptAuditIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}
