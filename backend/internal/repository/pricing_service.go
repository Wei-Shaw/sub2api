package repository

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type pricingRemoteClient struct {
	httpClient *http.Client
REDACTED

// pricingRemoteClientError 代理初始化失败时的错误占位客户端
// 所有请求直接返回初始化错误，禁止回退到直连
type pricingRemoteClientError struct {
	err error
REDACTED

func (c *pricingRemoteClientError) FetchPricingJSON(_ context.Context, _ string) ([]byte, error) {
	return nil, c.err
REDACTED

func (c *pricingRemoteClientError) FetchHashText(_ context.Context, _ string) (string, error) {
	return "", c.err
REDACTED

// NewPricingRemoteClient 创建定价数据远程客户端
// proxyURL 为空时直连，支持 http/https/socks5/socks5h 协议
// 代理配置失败时行为由 allowDirectOnProxyError 控制：
//   - false（默认）：返回错误占位客户端，禁止回退到直连
//   - true：回退到直连（仅限管理员显式开启）
func NewPricingRemoteClient(proxyURL string, allowDirectOnProxyError bool) service.PricingRemoteClient {
	// 安全说明：httpclient.GetClient 的错误链（url.Parse / proxyutil）不含明文代理凭据，
	// 但仍通过 slog 仅在服务端日志记录，不会暴露给 HTTP 响应。
	sharedClient, err := httpclient.GetClient(httpclient.Options{
		Timeout:  30 * time.Second,
		ProxyURL: proxyURL,
REDACTED)
	if err != nil {
		if strings.TrimSpace(proxyURL) != "" && !allowDirectOnProxyError {
			slog.Warn("proxy client init failed, all requests will fail", "service", "pricing", "error", err)
			return &pricingRemoteClientError{err: fmt.Errorf("proxy client init failed and direct fallback is disabled; set security.proxy_fallback.allow_direct_on_error=true to allow fallback: %w", err)REDACTED
	REDACTED
		sharedClient = &http.Client{Timeout: 30 * time.SecondREDACTED
REDACTED
	return &pricingRemoteClient{
		httpClient: sharedClient,
REDACTED
REDACTED

func (c *pricingRemoteClient) FetchPricingJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	return io.ReadAll(resp.Body)
REDACTED

func (c *pricingRemoteClient) FetchHashText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
REDACTED

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
REDACTED

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
REDACTED

	// 哈希文件格式：hash  filename 或者纯 hash
	hash := strings.TrimSpace(string(body))
	parts := strings.Fields(hash)
	if len(parts) > 0 {
		return parts[0], nil
REDACTED
	return hash, nil
REDACTED
