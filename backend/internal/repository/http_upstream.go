package repository

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// httpUpstreamService 通用 HTTP 上游服务
// 用于向任意 HTTP API（Claude、OpenAI 等）发送请求，支持可选代理
//
// 性能优化：
// 1. 使用 sync.Map 缓存代理客户端实例，避免每次请求都创建新的 http.Client
// 2. 复用 Transport 连接池，减少 TCP 握手和 TLS 协商开销
// 3. 原实现每次请求都 new 一个 http.Client，导致连接无法复用
type httpUpstreamService struct {
	// defaultClient: 无代理时使用的默认客户端（单例复用）
	defaultClient *http.Client
	// proxyClients: 按代理 URL 缓存的客户端池，避免重复创建
	proxyClients sync.Map
	cfg          *config.Config
REDACTED

// NewHTTPUpstream 创建通用 HTTP 上游服务
// 使用配置中的连接池参数构建 Transport
func NewHTTPUpstream(cfg *config.Config) service.HTTPUpstream {
	return &httpUpstreamService{
		defaultClient: &http.Client{Transport: buildUpstreamTransport(cfg, nil)REDACTED,
		cfg:           cfg,
REDACTED
REDACTED

func (s *httpUpstreamService) Do(req *http.Request, proxyURL string) (*http.Response, error) {
	if strings.TrimSpace(proxyURL) == "" {
		return s.defaultClient.Do(req)
REDACTED
	client := s.getOrCreateClient(proxyURL)
	return client.Do(req)
REDACTED

// getOrCreateClient 获取或创建代理客户端
// 性能优化：使用 sync.Map 实现无锁缓存，相同代理 URL 复用同一客户端
// LoadOrStore 保证并发安全，避免重复创建
func (s *httpUpstreamService) getOrCreateClient(proxyURL string) *http.Client {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return s.defaultClient
REDACTED
	// 优先从缓存获取，命中则直接返回
	if cached, ok := s.proxyClients.Load(proxyURL); ok {
		return cached.(*http.Client)
REDACTED

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return s.defaultClient
REDACTED

	// 创建新客户端并缓存，LoadOrStore 保证只有一个实例被存储
	client := &http.Client{Transport: buildUpstreamTransport(s.cfg, parsedURL)REDACTED
	actual, _ := s.proxyClients.LoadOrStore(proxyURL, client)
	return actual.(*http.Client)
REDACTED

// buildUpstreamTransport 构建上游请求的 Transport
// 使用配置文件中的连接池参数，支持生产环境调优
func buildUpstreamTransport(cfg *config.Config, proxyURL *url.URL) *http.Transport {
	// 读取配置，使用合理的默认值
	maxIdleConns := cfg.Gateway.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 240
REDACTED
	maxIdleConnsPerHost := cfg.Gateway.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = 120
REDACTED
	maxConnsPerHost := cfg.Gateway.MaxConnsPerHost
	if maxConnsPerHost < 0 {
		maxConnsPerHost = 240
REDACTED
	idleConnTimeout := time.Duration(cfg.Gateway.IdleConnTimeoutSeconds) * time.Second
	if idleConnTimeout <= 0 {
		idleConnTimeout = 300 * time.Second
REDACTED
	responseHeaderTimeout := time.Duration(cfg.Gateway.ResponseHeaderTimeout) * time.Second
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = 300 * time.Second
REDACTED

	transport := &http.Transport{
		MaxIdleConns:          maxIdleConns,        // 最大空闲连接总数
		MaxIdleConnsPerHost:   maxIdleConnsPerHost, // 每主机最大空闲连接
		MaxConnsPerHost:       maxConnsPerHost,     // 每主机最大连接数（含活跃）
		IdleConnTimeout:       idleConnTimeout,     // 空闲连接超时
		ResponseHeaderTimeout: responseHeaderTimeout,
REDACTED
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
REDACTED
	return transport
REDACTED
