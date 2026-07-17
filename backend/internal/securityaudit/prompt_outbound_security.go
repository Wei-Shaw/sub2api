package securityaudit

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const maxGuardResponseBytes int64 = 256 * 1024

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
REDACTED
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url_scheme", "审计节点仅支持 HTTP(S)")
REDACTED
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", infraerrors.BadRequest("prompt_audit_unsafe_base_url", "审计节点地址不能包含凭据、查询参数或片段")
REDACTED
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
REDACTED
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.EqualFold(path, "/v1") {
		path = ""
REDACTED
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
REDACTED

func ChatCompletionsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
REDACTED
	return normalized + "/v1/chat/completions", nil
REDACTED

func ModelsURL(base string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
REDACTED
	return normalized + "/v1/models", nil
REDACTED

func NewSecureHTTPClient(endpoint ActiveEndpoint) (*http.Client, error) {
	_, err := NormalizeBaseURL(endpoint.BaseURL)
	if err != nil {
		return nil, err
REDACTED
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.SecondREDACTED
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
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12REDACTED,
REDACTED
	// Endpoint ownership and destination trust are administrator concerns.
	// Use the standard dialer so configured private, loopback, reserved, and
	// DNS-resolved addresses are all reachable from the service environment.
	transport.DialContext = dialer.DialContext
	timeout := time.Duration(endpoint.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeoutMS * time.Millisecond
REDACTED
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
REDACTED, nil
REDACTED
