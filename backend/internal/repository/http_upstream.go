package repository

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// 默认配置常量
// 这些值在配置文件未指定时作为回退默认值使用
const (
	// directProxyKey: 无代理时的缓存键标识
	directProxyKey = "direct"
	// defaultMaxIdleConns: 默认最大空闲连接总数
	// HTTP/2 场景下，单连接可多路复用，240 足以支撑高并发
	defaultMaxIdleConns = 240
	// defaultMaxIdleConnsPerHost: 默认每主机最大空闲连接数
	defaultMaxIdleConnsPerHost = 120
	// defaultMaxConnsPerHost: 默认每主机最大连接数（含活跃连接）
	// 达到上限后新请求会等待，而非无限创建连接
	defaultMaxConnsPerHost = 240
	// defaultIdleConnTimeout: 默认空闲连接超时时间（90秒）
	// 超时后连接会被关闭，释放系统资源（建议小于上游 LB 超时）
	defaultIdleConnTimeout = 90 * time.Second
	// defaultResponseHeaderTimeout: 默认等待响应头超时时间（5分钟）
	// LLM 请求可能排队较久，需要较长超时
	defaultResponseHeaderTimeout = 300 * time.Second
	// defaultMaxUpstreamClients: 默认最大客户端缓存数量
	// 超出后会淘汰最久未使用的客户端
	defaultMaxUpstreamClients = 5000
	// defaultClientIdleTTLSeconds: 默认客户端空闲回收阈值（15分钟）
	defaultClientIdleTTLSeconds = 900
	// OpenAI HTTP/2 代理回退策略默认值
	defaultOpenAIHTTP2FallbackErrorThreshold = 2
	defaultOpenAIHTTP2FallbackWindow         = 60 * time.Second
	defaultOpenAIHTTP2FallbackTTL            = 10 * time.Minute
)

const (
	upstreamProtocolModeDefault          = "default"
	upstreamProtocolModeOpenAIH1         = "openai_h1"
	upstreamProtocolModeOpenAIH2         = "openai_h2"
	upstreamProtocolModeOpenAIH1Fallback = "openai_h1_fallback"
)

var errUpstreamClientLimitReached = errors.New("upstream client cache limit reached")

// poolSettings 连接池配置参数
// 封装 Transport 所需的各项连接池参数
type poolSettings struct {
	maxIdleConns          int           // 最大空闲连接总数
	maxIdleConnsPerHost   int           // 每主机最大空闲连接数
	maxConnsPerHost       int           // 每主机最大连接数（含活跃）
	idleConnTimeout       time.Duration // 空闲连接超时时间
	responseHeaderTimeout time.Duration // 等待响应头超时时间
REDACTED

type openAIHTTP2Settings struct {
	enabled                   bool
	allowProxyFallbackToHTTP1 bool
	fallbackErrorThreshold    int
	fallbackWindow            time.Duration
	fallbackTTL               time.Duration
REDACTED

// upstreamClientEntry 上游客户端缓存条目
// 记录客户端实例及其元数据，用于连接池管理和淘汰策略
type upstreamClientEntry struct {
	client       *http.Client // HTTP 客户端实例
	proxyKey     string       // 代理标识（用于检测代理变更）
	poolKey      string       // 连接池配置标识（用于检测配置变更）
	protocolMode string       // 协议模式（default/openai_h1/openai_h2/openai_h1_fallback）
	lastUsed     int64        // 最后使用时间戳（纳秒），用于 LRU 淘汰
	inFlight     int64        // 当前进行中的请求数，>0 时不可淘汰
REDACTED

type openAIHTTP2FallbackState struct {
	mu            sync.Mutex
	windowStart   time.Time
	errorCount    int
	fallbackUntil time.Time
REDACTED

// httpUpstreamService 通用 HTTP 上游服务
// 用于向任意 HTTP API（Claude、OpenAI 等）发送请求，支持可选代理
//
// 架构设计：
// - 根据隔离策略（proxy/account/account_proxy）缓存客户端实例
// - 每个客户端拥有独立的 Transport 连接池
// - 支持 LRU + 空闲时间双重淘汰策略
//
// 性能优化：
// 1. 根据隔离策略缓存客户端实例，避免频繁创建 http.Client
// 2. 复用 Transport 连接池，减少 TCP 握手和 TLS 协商开销
// 3. 支持账号级隔离与空闲回收，降低连接层关联风险
// 4. 达到最大连接数后等待可用连接，而非无限创建
// 5. 仅回收空闲客户端，避免中断活跃请求
// 6. HTTP/2 多路复用，连接上限不等于并发请求上限
// 7. 代理变更时清空旧连接池，避免复用错误代理
// 8. 账号并发数与连接池上限对应（账号隔离策略下）
type httpUpstreamService struct {
	cfg     *config.Config                  // 全局配置
	mu      sync.RWMutex                    // 保护 clients map 的读写锁
	clients map[string]*upstreamClientEntry // 客户端缓存池，key 由隔离策略决定
	// OpenAI 走 HTTP/HTTPS 代理时的 H2->H1 回退状态（key=标准化 proxyKey）
	openAIHTTP2Fallbacks sync.Map
REDACTED

// NewHTTPUpstream 创建通用 HTTP 上游服务
// 使用配置中的连接池参数构建 Transport
//
// 参数:
//   - cfg: 全局配置，包含连接池参数和隔离策略
//
// 返回:
//   - service.HTTPUpstream 接口实现
func NewHTTPUpstream(cfg *config.Config) service.HTTPUpstream {
	return &httpUpstreamService{
		cfg:     cfg,
		clients: make(map[string]*upstreamClientEntry),
REDACTED
REDACTED

// Do 执行 HTTP 请求
// 根据隔离策略获取或创建客户端，并跟踪请求生命周期
//
// 参数:
//   - req: HTTP 请求对象
//   - proxyURL: 代理地址，空字符串表示直连
//   - accountID: 账户 ID，用于账户级隔离
//   - accountConcurrency: 账户并发限制，用于动态调整连接池大小
//
// 返回:
//   - *http.Response: HTTP 响应（Body 已包装，关闭时自动更新计数）
//   - error: 请求错误
//
// 注意:
//   - 调用方必须关闭 resp.Body，否则会导致 inFlight 计数泄漏
//   - inFlight > 0 的客户端不会被淘汰，确保活跃请求不被中断
func (s *httpUpstreamService) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	if err := s.validateRequestHost(req); err != nil {
		return nil, err
REDACTED
	profile := service.HTTPUpstreamProfileDefault
	if req != nil {
		profile = service.HTTPUpstreamProfileFromContext(req.Context())
REDACTED

	// 获取或创建对应的客户端，并标记请求占用
	entry, err := s.acquireClientWithProfile(proxyURL, accountID, accountConcurrency, profile)
	if err != nil {
		return nil, err
REDACTED

	// 执行请求
	resp, err := entry.client.Do(req)
	if err != nil {
		s.recordOpenAIHTTP2Failure(profile, entry.protocolMode, entry.proxyKey, err)
		// 请求失败，立即减少计数
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
		return nil, err
REDACTED
	s.recordOpenAIHTTP2Success(profile, entry.protocolMode, entry.proxyKey)

	// 如果上游返回了压缩内容，解压后再交给业务层
	decompressResponseBody(resp)

	// 包装响应体，在关闭时自动减少计数并更新时间戳
	// 这确保了流式响应（如 SSE）在完全读取前不会被淘汰
	resp.Body = wrapTrackedBody(resp.Body, func() {
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
REDACTED)

	return resp, nil
REDACTED

// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
//
// profile 为 nil 时不启用 TLS 指纹，行为与 Do 方法相同。
// profile 非 nil 时使用指定的 Profile 进行 TLS 指纹伪装。
func (s *httpUpstreamService) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if profile == nil {
		return s.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED
	upstreamProfile := service.HTTPUpstreamProfileDefault
	if req != nil {
		upstreamProfile = service.HTTPUpstreamProfileFromContext(req.Context())
REDACTED

	targetHost := ""
	if req != nil && req.URL != nil {
		targetHost = req.URL.Host
REDACTED
	proxyInfo := "direct"
	if proxyURL != "" {
		proxyInfo = proxyURL
REDACTED
	slog.Debug("tls_fingerprint_enabled", "account_id", accountID, "target", targetHost, "proxy", proxyInfo, "profile", profile.Name)

	if err := s.validateRequestHost(req); err != nil {
		return nil, err
REDACTED

	entry, err := s.acquireClientWithTLS(proxyURL, accountID, accountConcurrency, profile, upstreamProfile)
	if err != nil {
		slog.Debug("tls_fingerprint_acquire_client_failed", "account_id", accountID, "error", err)
		return nil, err
REDACTED

	resp, err := entry.client.Do(req)
	if err != nil {
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
		slog.Debug("tls_fingerprint_request_failed", "account_id", accountID, "error", err)
		return nil, err
REDACTED

	decompressResponseBody(resp)

	resp.Body = wrapTrackedBody(resp.Body, func() {
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
REDACTED)

	return resp, nil
REDACTED

// acquireClientWithTLS 获取或创建带 TLS 指纹的客户端
func (s *httpUpstreamService) acquireClientWithTLS(proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile, upstreamProfile service.HTTPUpstreamProfile) (*upstreamClientEntry, error) {
	return s.getClientEntryWithTLS(proxyURL, accountID, accountConcurrency, profile, upstreamProfile, true, true)
REDACTED

// getClientEntryWithTLS 获取或创建带 TLS 指纹的客户端条目
// TLS 指纹客户端使用独立的缓存键，与普通客户端隔离
func (s *httpUpstreamService) getClientEntryWithTLS(proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile, upstreamProfile service.HTTPUpstreamProfile, markInFlight bool, enforceLimit bool) (*upstreamClientEntry, error) {
	isolation := s.getIsolationMode()
	proxyKey, parsedProxy, err := normalizeProxyURL(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	settings := s.resolvePoolSettings(isolation, accountConcurrency)
	settings = s.applyProfilePoolSettings(settings, upstreamProfile)
	// TLS 指纹客户端使用独立的缓存键，加 "tls:" 前缀
	cacheKey := "tls:" + buildCacheKey(isolation, proxyKey, accountID, upstreamProtocolModeDefault)
	poolKey := buildPoolKey(settings, upstreamProtocolModeDefault) + ":tls"

	now := time.Now()
	nowUnix := now.UnixNano()

	// 读锁快速路径
	s.mu.RLock()
	if entry, ok := s.clients[cacheKey]; ok && s.shouldReuseEntry(entry, isolation, proxyKey, poolKey) {
		atomic.StoreInt64(&entry.lastUsed, nowUnix)
		if markInFlight {
			atomic.AddInt64(&entry.inFlight, 1)
	REDACTED
		s.mu.RUnlock()
		slog.Debug("tls_fingerprint_reusing_client", "account_id", accountID, "cache_key", cacheKey)
		return entry, nil
REDACTED
	s.mu.RUnlock()

	// 写锁慢路径
	s.mu.Lock()
	if entry, ok := s.clients[cacheKey]; ok {
		if s.shouldReuseEntry(entry, isolation, proxyKey, poolKey) {
			atomic.StoreInt64(&entry.lastUsed, nowUnix)
			if markInFlight {
				atomic.AddInt64(&entry.inFlight, 1)
		REDACTED
			s.mu.Unlock()
			slog.Debug("tls_fingerprint_reusing_client", "account_id", accountID, "cache_key", cacheKey)
			return entry, nil
	REDACTED
		slog.Debug("tls_fingerprint_evicting_stale_client",
			"account_id", accountID,
			"cache_key", cacheKey,
			"proxy_changed", entry.proxyKey != proxyKey,
			"pool_changed", entry.poolKey != poolKey)
		s.removeClientLocked(cacheKey, entry)
REDACTED

	// 超出缓存上限时尝试淘汰
	if enforceLimit && s.maxUpstreamClients() > 0 {
		s.evictIdleLocked(now)
		if len(s.clients) >= s.maxUpstreamClients() {
			if !s.evictOldestIdleLocked() {
				s.mu.Unlock()
				return nil, errUpstreamClientLimitReached
		REDACTED
	REDACTED
REDACTED

	// 创建带 TLS 指纹的 Transport
	slog.Debug("tls_fingerprint_creating_new_client", "account_id", accountID, "cache_key", cacheKey, "proxy", proxyKey)
	transport, err := buildUpstreamTransportWithTLSFingerprint(settings, parsedProxy, profile)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("build TLS fingerprint transport: %w", err)
REDACTED

	client := &http.Client{Transport: transportREDACTED
	if s.shouldValidateResolvedIP() {
		client.CheckRedirect = s.redirectChecker
REDACTED

	entry := &upstreamClientEntry{
		client:   client,
		proxyKey: proxyKey,
		poolKey:  poolKey,
REDACTED
	atomic.StoreInt64(&entry.lastUsed, nowUnix)
	if markInFlight {
		atomic.StoreInt64(&entry.inFlight, 1)
REDACTED
	s.clients[cacheKey] = entry

	s.evictIdleLocked(now)
	s.evictOverLimitLocked()
	s.mu.Unlock()
	return entry, nil
REDACTED

func (s *httpUpstreamService) shouldValidateResolvedIP() bool {
	if s.cfg == nil {
		return false
REDACTED
	if !s.cfg.Security.URLAllowlist.Enabled {
		return false
REDACTED
	return !s.cfg.Security.URLAllowlist.AllowPrivateHosts
REDACTED

func (s *httpUpstreamService) validateRequestHost(req *http.Request) error {
	if !s.shouldValidateResolvedIP() {
		return nil
REDACTED
	if req == nil || req.URL == nil {
		return errors.New("request url is nil")
REDACTED
	host := strings.TrimSpace(req.URL.Hostname())
	if host == "" {
		return errors.New("request host is empty")
REDACTED
	if err := urlvalidator.ValidateResolvedIP(host); err != nil {
		return err
REDACTED
	return nil
REDACTED

func (s *httpUpstreamService) redirectChecker(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
REDACTED
	return s.validateRequestHost(req)
REDACTED

// acquireClient 获取或创建客户端，并标记为进行中请求
// 用于请求路径，避免在获取后被淘汰
func (s *httpUpstreamService) acquireClient(proxyURL string, accountID int64, accountConcurrency int) (*upstreamClientEntry, error) {
	return s.acquireClientWithProfile(proxyURL, accountID, accountConcurrency, service.HTTPUpstreamProfileDefault)
REDACTED

// acquireClientWithProfile 获取或创建客户端，并按请求 profile 选择协议策略。
func (s *httpUpstreamService) acquireClientWithProfile(proxyURL string, accountID int64, accountConcurrency int, profile service.HTTPUpstreamProfile) (*upstreamClientEntry, error) {
	return s.getClientEntry(proxyURL, accountID, accountConcurrency, profile, true, true)
REDACTED

// getOrCreateClient 获取或创建客户端
// 根据隔离策略和参数决定缓存键，处理代理变更和配置变更
//
// 参数:
//   - proxyURL: 代理地址
//   - accountID: 账户 ID
//   - accountConcurrency: 账户并发限制
//
// 返回:
//   - *upstreamClientEntry: 客户端缓存条目
//
// 隔离策略说明:
//   - proxy: 按代理地址隔离，同一代理共享客户端
//   - account: 按账户隔离，同一账户共享客户端（代理变更时重建）
//   - account_proxy: 按账户+代理组合隔离，最细粒度
func (s *httpUpstreamService) getOrCreateClient(proxyURL string, accountID int64, accountConcurrency int) (*upstreamClientEntry, error) {
	return s.getClientEntry(proxyURL, accountID, accountConcurrency, service.HTTPUpstreamProfileDefault, false, false)
REDACTED

// getClientEntry 获取或创建客户端条目
// markInFlight=true 时会标记进行中请求，用于请求路径防止被淘汰
// enforceLimit=true 时会限制客户端数量，超限且无法淘汰时返回错误
func (s *httpUpstreamService) getClientEntry(proxyURL string, accountID int64, accountConcurrency int, profile service.HTTPUpstreamProfile, markInFlight bool, enforceLimit bool) (*upstreamClientEntry, error) {
	// 获取隔离模式
	isolation := s.getIsolationMode()
	// 标准化代理 URL 并解析
	proxyKey, parsedProxy, err := normalizeProxyURL(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	// 根据请求 profile（例如 OpenAI）选择协议模式
	protocolMode := s.resolveProtocolMode(profile, proxyKey, parsedProxy)
	settings := s.resolvePoolSettings(isolation, accountConcurrency)
	settings = s.applyProfilePoolSettings(settings, profile)
	// 构建缓存键（根据隔离策略不同）
	cacheKey := buildCacheKey(isolation, proxyKey, accountID, protocolMode)
	// 构建连接池配置键（用于检测配置变更）
	poolKey := buildPoolKey(settings, protocolMode)

	now := time.Now()
	nowUnix := now.UnixNano()

	// 读锁快速路径：命中缓存直接返回，减少锁竞争
	s.mu.RLock()
	if entry, ok := s.clients[cacheKey]; ok && s.shouldReuseEntry(entry, isolation, proxyKey, poolKey) {
		atomic.StoreInt64(&entry.lastUsed, nowUnix)
		if markInFlight {
			atomic.AddInt64(&entry.inFlight, 1)
	REDACTED
		s.mu.RUnlock()
		return entry, nil
REDACTED
	s.mu.RUnlock()

	// 写锁慢路径：创建或重建客户端
	s.mu.Lock()
	if entry, ok := s.clients[cacheKey]; ok {
		if s.shouldReuseEntry(entry, isolation, proxyKey, poolKey) {
			atomic.StoreInt64(&entry.lastUsed, nowUnix)
			if markInFlight {
				atomic.AddInt64(&entry.inFlight, 1)
		REDACTED
			s.mu.Unlock()
			return entry, nil
	REDACTED
		s.removeClientLocked(cacheKey, entry)
REDACTED

	// 超出缓存上限时尝试淘汰，无法淘汰则拒绝新建
	if enforceLimit && s.maxUpstreamClients() > 0 {
		s.evictIdleLocked(now)
		if len(s.clients) >= s.maxUpstreamClients() {
			if !s.evictOldestIdleLocked() {
				s.mu.Unlock()
				return nil, errUpstreamClientLimitReached
		REDACTED
	REDACTED
REDACTED

	// 缓存未命中或需要重建，创建新客户端
	transport, err := buildUpstreamTransport(settings, parsedProxy, protocolMode)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("build transport: %w", err)
REDACTED
	client := &http.Client{Transport: transportREDACTED
	if s.shouldValidateResolvedIP() {
		client.CheckRedirect = s.redirectChecker
REDACTED
	entry := &upstreamClientEntry{
		client:       client,
		proxyKey:     proxyKey,
		poolKey:      poolKey,
		protocolMode: protocolMode,
REDACTED
	atomic.StoreInt64(&entry.lastUsed, nowUnix)
	if markInFlight {
		atomic.StoreInt64(&entry.inFlight, 1)
REDACTED
	s.clients[cacheKey] = entry

	// 执行淘汰策略：先淘汰空闲超时的，再淘汰超出数量限制的
	s.evictIdleLocked(now)
	s.evictOverLimitLocked()
	s.mu.Unlock()
	return entry, nil
REDACTED

// shouldReuseEntry 判断缓存条目是否可复用
// 若代理或连接池配置发生变化，则需要重建客户端
func (s *httpUpstreamService) shouldReuseEntry(entry *upstreamClientEntry, isolation, proxyKey, poolKey string) bool {
	if entry == nil {
		return false
REDACTED
	if isolation == config.ConnectionPoolIsolationAccount && entry.proxyKey != proxyKey {
		return false
REDACTED
	if entry.poolKey != poolKey {
		return false
REDACTED
	return true
REDACTED

// removeClientLocked 移除客户端（需持有锁）
// 从缓存中删除并关闭空闲连接
//
// 参数:
//   - key: 缓存键
//   - entry: 客户端条目
func (s *httpUpstreamService) removeClientLocked(key string, entry *upstreamClientEntry) {
	delete(s.clients, key)
	if entry != nil && entry.client != nil {
		// 关闭空闲连接，释放系统资源
		// 注意：这不会中断活跃连接
		entry.client.CloseIdleConnections()
REDACTED
REDACTED

// evictIdleLocked 淘汰空闲超时的客户端（需持有锁）
// 遍历所有客户端，移除超过 TTL 且无活跃请求的条目
//
// 参数:
//   - now: 当前时间
func (s *httpUpstreamService) evictIdleLocked(now time.Time) {
	ttl := s.clientIdleTTL()
	if ttl <= 0 {
		return
REDACTED
	// 计算淘汰截止时间
	cutoff := now.Add(-ttl).UnixNano()
	for key, entry := range s.clients {
		// 跳过有活跃请求的客户端
		if atomic.LoadInt64(&entry.inFlight) != 0 {
			continue
	REDACTED
		// 淘汰超时的空闲客户端
		if atomic.LoadInt64(&entry.lastUsed) <= cutoff {
			s.removeClientLocked(key, entry)
	REDACTED
REDACTED
REDACTED

// evictOldestIdleLocked 淘汰最久未使用且无活跃请求的客户端（需持有锁）
func (s *httpUpstreamService) evictOldestIdleLocked() bool {
	var (
		oldestKey   string
		oldestEntry *upstreamClientEntry
		oldestTime  int64
	)
	// 查找最久未使用且无活跃请求的客户端
	for key, entry := range s.clients {
		// 跳过有活跃请求的客户端
		if atomic.LoadInt64(&entry.inFlight) != 0 {
			continue
	REDACTED
		lastUsed := atomic.LoadInt64(&entry.lastUsed)
		if oldestEntry == nil || lastUsed < oldestTime {
			oldestKey = key
			oldestEntry = entry
			oldestTime = lastUsed
	REDACTED
REDACTED
	// 所有客户端都有活跃请求，无法淘汰
	if oldestEntry == nil {
		return false
REDACTED
	s.removeClientLocked(oldestKey, oldestEntry)
	return true
REDACTED

// evictOverLimitLocked 淘汰超出数量限制的客户端（需持有锁）
// 使用 LRU 策略，优先淘汰最久未使用且无活跃请求的客户端
func (s *httpUpstreamService) evictOverLimitLocked() bool {
	maxClients := s.maxUpstreamClients()
	if maxClients <= 0 {
		return false
REDACTED
	evicted := false
	// 循环淘汰直到满足数量限制
	for len(s.clients) > maxClients {
		if !s.evictOldestIdleLocked() {
			return evicted
	REDACTED
		evicted = true
REDACTED
	return evicted
REDACTED

// getIsolationMode 获取连接池隔离模式
// 从配置中读取，无效值回退到 account_proxy 模式
//
// 返回:
//   - string: 隔离模式（proxy/account/account_proxy）
func (s *httpUpstreamService) getIsolationMode() string {
	if s.cfg == nil {
		return config.ConnectionPoolIsolationAccountProxy
REDACTED
	mode := strings.ToLower(strings.TrimSpace(s.cfg.Gateway.ConnectionPoolIsolation))
	if mode == "" {
		return config.ConnectionPoolIsolationAccountProxy
REDACTED
	switch mode {
	case config.ConnectionPoolIsolationProxy, config.ConnectionPoolIsolationAccount, config.ConnectionPoolIsolationAccountProxy:
		return mode
	default:
		return config.ConnectionPoolIsolationAccountProxy
REDACTED
REDACTED

// maxUpstreamClients 获取最大客户端缓存数量
// 从配置中读取，无效值使用默认值
func (s *httpUpstreamService) maxUpstreamClients() int {
	if s.cfg == nil {
		return defaultMaxUpstreamClients
REDACTED
	if s.cfg.Gateway.MaxUpstreamClients > 0 {
		return s.cfg.Gateway.MaxUpstreamClients
REDACTED
	return defaultMaxUpstreamClients
REDACTED

// clientIdleTTL 获取客户端空闲回收阈值
// 从配置中读取，无效值使用默认值
func (s *httpUpstreamService) clientIdleTTL() time.Duration {
	if s.cfg == nil {
		return time.Duration(defaultClientIdleTTLSeconds) * time.Second
REDACTED
	if s.cfg.Gateway.ClientIdleTTLSeconds > 0 {
		return time.Duration(s.cfg.Gateway.ClientIdleTTLSeconds) * time.Second
REDACTED
	return time.Duration(defaultClientIdleTTLSeconds) * time.Second
REDACTED

// resolvePoolSettings 解析连接池配置
// 根据隔离策略和账户并发数动态调整连接池参数
//
// 参数:
//   - isolation: 隔离模式
//   - accountConcurrency: 账户并发限制
//
// 返回:
//   - poolSettings: 连接池配置
//
// 说明:
//   - 账户隔离模式下，连接池大小与账户并发数对应
//   - 这确保了单账户不会占用过多连接资源
func (s *httpUpstreamService) resolvePoolSettings(isolation string, accountConcurrency int) poolSettings {
	settings := defaultPoolSettings(s.cfg)
	// 账户隔离模式下，根据账户并发数调整连接池大小
	if (isolation == config.ConnectionPoolIsolationAccount || isolation == config.ConnectionPoolIsolationAccountProxy) && accountConcurrency > 0 {
		settings.maxIdleConns = accountConcurrency
		settings.maxIdleConnsPerHost = accountConcurrency
		settings.maxConnsPerHost = accountConcurrency
REDACTED
	return settings
REDACTED

func (s *httpUpstreamService) applyProfilePoolSettings(settings poolSettings, profile service.HTTPUpstreamProfile) poolSettings {
	if profile != service.HTTPUpstreamProfileOpenAI {
		return settings
REDACTED
	settings.responseHeaderTimeout = 0
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIResponseHeaderTimeout > 0 {
		settings.responseHeaderTimeout = time.Duration(s.cfg.Gateway.OpenAIResponseHeaderTimeout) * time.Second
REDACTED
	return settings
REDACTED

// buildPoolKey 构建连接池配置键，用于检测连接池配置变更。
func buildPoolKey(settings poolSettings, protocolMode string) string {
	base := fmt.Sprintf(
		"idle:%d|idle_host:%d|max:%d|idle_timeout:%s|header_timeout:%s",
		settings.maxIdleConns,
		settings.maxIdleConnsPerHost,
		settings.maxConnsPerHost,
		settings.idleConnTimeout,
		settings.responseHeaderTimeout,
	)
	if protocolMode == "" || protocolMode == upstreamProtocolModeDefault {
		return base
REDACTED
	return base + "|proto:" + protocolMode
REDACTED

// buildCacheKey 构建客户端缓存键
// 根据隔离策略决定缓存键的组成
//
// 参数:
//   - isolation: 隔离模式
//   - proxyKey: 代理标识
//   - accountID: 账户 ID
//
// 返回:
//   - string: 缓存键
//
// 缓存键格式:
//   - proxy 模式: "proxy:{proxyKeyREDACTED"
//   - account 模式: "account:{accountIDREDACTED"
//   - account_proxy 模式: "account:{accountIDREDACTED|proxy:{proxyKeyREDACTED"
func buildCacheKey(isolation, proxyKey string, accountID int64, protocolMode string) string {
	var base string
	switch isolation {
	case config.ConnectionPoolIsolationAccount:
		base = fmt.Sprintf("account:%d", accountID)
	case config.ConnectionPoolIsolationAccountProxy:
		base = fmt.Sprintf("account:%d|proxy:%s", accountID, proxyKey)
	default:
		base = fmt.Sprintf("proxy:%s", proxyKey)
REDACTED
	if protocolMode != "" && protocolMode != upstreamProtocolModeDefault {
		base += "|proto:" + protocolMode
REDACTED
	return base
REDACTED

func (s *httpUpstreamService) resolveOpenAIHTTP2Settings() openAIHTTP2Settings {
	settings := openAIHTTP2Settings{
		enabled:                   false,
		allowProxyFallbackToHTTP1: true,
		fallbackErrorThreshold:    defaultOpenAIHTTP2FallbackErrorThreshold,
		fallbackWindow:            defaultOpenAIHTTP2FallbackWindow,
		fallbackTTL:               defaultOpenAIHTTP2FallbackTTL,
REDACTED
	if s == nil || s.cfg == nil {
		return settings
REDACTED
	cfg := s.cfg.Gateway.OpenAIHTTP2
	settings.enabled = cfg.Enabled
	settings.allowProxyFallbackToHTTP1 = cfg.AllowProxyFallbackToHTTP1
	if cfg.FallbackErrorThreshold > 0 {
		settings.fallbackErrorThreshold = cfg.FallbackErrorThreshold
REDACTED
	if cfg.FallbackWindowSeconds > 0 {
		settings.fallbackWindow = time.Duration(cfg.FallbackWindowSeconds) * time.Second
REDACTED
	if cfg.FallbackTTLSeconds > 0 {
		settings.fallbackTTL = time.Duration(cfg.FallbackTTLSeconds) * time.Second
REDACTED
	return settings
REDACTED

func (s *httpUpstreamService) resolveProtocolMode(profile service.HTTPUpstreamProfile, proxyKey string, parsedProxy *url.URL) string {
	if profile != service.HTTPUpstreamProfileOpenAI {
		return upstreamProtocolModeDefault
REDACTED
	settings := s.resolveOpenAIHTTP2Settings()
	if !settings.enabled {
		return upstreamProtocolModeOpenAIH1
REDACTED
	if parsedProxy == nil {
		return upstreamProtocolModeOpenAIH2
REDACTED
	scheme := strings.ToLower(parsedProxy.Scheme)
	if scheme != "http" && scheme != "https" {
		return upstreamProtocolModeOpenAIH2
REDACTED
	if settings.allowProxyFallbackToHTTP1 && s.isOpenAIHTTP2FallbackActive(proxyKey) {
		return upstreamProtocolModeOpenAIH1Fallback
REDACTED
	return upstreamProtocolModeOpenAIH2
REDACTED

func (s *httpUpstreamService) isOpenAIHTTP2FallbackActive(proxyKey string) bool {
	raw, ok := s.openAIHTTP2Fallbacks.Load(proxyKey)
	if !ok {
		return false
REDACTED
	state, ok := raw.(*openAIHTTP2FallbackState)
	if !ok || state == nil {
		return false
REDACTED
	return state.isFallbackActive(time.Now())
REDACTED

func (s *httpUpstreamService) getOrCreateOpenAIHTTP2FallbackState(proxyKey string) *openAIHTTP2FallbackState {
	state := &openAIHTTP2FallbackState{REDACTED
	actual, _ := s.openAIHTTP2Fallbacks.LoadOrStore(proxyKey, state)
	cached, ok := actual.(*openAIHTTP2FallbackState)
	if !ok || cached == nil {
		return state
REDACTED
	return cached
REDACTED

func isHTTPProxyKey(proxyKey string) bool {
	return strings.HasPrefix(proxyKey, "http://") || strings.HasPrefix(proxyKey, "https://")
REDACTED

func isOpenAIHTTP2CompatibilityError(err error) bool {
	if err == nil {
		return false
REDACTED
	if isUpstreamTimeoutError(err) {
		return false
REDACTED
	msg := strings.ToLower(err.Error())
	if msg == "" {
		return false
REDACTED
	markers := []string{
		"alpn",
		"no application protocol",
		"protocol error",
		"stream error",
		"goaway",
		"refused_stream",
		"frame too large",
REDACTED
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func isUpstreamTimeoutError(err error) bool {
	if err == nil {
		return false
REDACTED
	if errors.Is(err, context.DeadlineExceeded) {
		return true
REDACTED
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
REDACTED
	msg := strings.ToLower(err.Error())
	if msg == "" {
		return false
REDACTED
	timeoutMarkers := []string{
		"timeout awaiting response headers",
		"i/o timeout",
		"context deadline exceeded",
		"client.timeout exceeded while awaiting headers",
		"tls handshake timeout",
REDACTED
	for _, marker := range timeoutMarkers {
		if strings.Contains(msg, marker) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *httpUpstreamService) recordOpenAIHTTP2Failure(profile service.HTTPUpstreamProfile, protocolMode, proxyKey string, err error) {
	if profile != service.HTTPUpstreamProfileOpenAI || protocolMode != upstreamProtocolModeOpenAIH2 {
		return
REDACTED
	settings := s.resolveOpenAIHTTP2Settings()
	if !settings.enabled || !settings.allowProxyFallbackToHTTP1 {
		return
REDACTED
	if !isHTTPProxyKey(proxyKey) || !isOpenAIHTTP2CompatibilityError(err) {
		return
REDACTED
	state := s.getOrCreateOpenAIHTTP2FallbackState(proxyKey)
	activated, until := state.recordFailure(time.Now(), settings.fallbackErrorThreshold, settings.fallbackWindow, settings.fallbackTTL)
	if activated {
		slog.Warn("openai_http2_proxy_fallback_activated",
			"proxy", proxyKey,
			"fallback_until", until.Format(time.RFC3339))
REDACTED
REDACTED

func (s *httpUpstreamService) recordOpenAIHTTP2Success(profile service.HTTPUpstreamProfile, protocolMode, proxyKey string) {
	if profile != service.HTTPUpstreamProfileOpenAI || protocolMode != upstreamProtocolModeOpenAIH2 {
		return
REDACTED
	if !isHTTPProxyKey(proxyKey) {
		return
REDACTED
	raw, ok := s.openAIHTTP2Fallbacks.Load(proxyKey)
	if !ok {
		return
REDACTED
	state, ok := raw.(*openAIHTTP2FallbackState)
	if !ok || state == nil {
		return
REDACTED
	state.resetErrorWindow()
REDACTED

func (s *openAIHTTP2FallbackState) isFallbackActive(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallbackUntil.IsZero() {
		return false
REDACTED
	if now.Before(s.fallbackUntil) {
		return true
REDACTED
	s.fallbackUntil = time.Time{REDACTED
	return false
REDACTED

func (s *openAIHTTP2FallbackState) resetErrorWindow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowStart = time.Time{REDACTED
	s.errorCount = 0
REDACTED

func (s *openAIHTTP2FallbackState) recordFailure(now time.Time, threshold int, window, ttl time.Duration) (bool, time.Time) {
	if threshold <= 0 {
		threshold = defaultOpenAIHTTP2FallbackErrorThreshold
REDACTED
	if window <= 0 {
		window = defaultOpenAIHTTP2FallbackWindow
REDACTED
	if ttl <= 0 {
		ttl = defaultOpenAIHTTP2FallbackTTL
REDACTED

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.fallbackUntil.IsZero() && now.Before(s.fallbackUntil) {
		return false, s.fallbackUntil
REDACTED
	if !s.fallbackUntil.IsZero() && !now.Before(s.fallbackUntil) {
		s.fallbackUntil = time.Time{REDACTED
REDACTED

	if s.windowStart.IsZero() || now.Sub(s.windowStart) > window {
		s.windowStart = now
		s.errorCount = 0
REDACTED
	s.errorCount++
	if s.errorCount < threshold {
		return false, time.Time{REDACTED
REDACTED

	s.fallbackUntil = now.Add(ttl)
	s.windowStart = time.Time{REDACTED
	s.errorCount = 0
	return true, s.fallbackUntil
REDACTED

// normalizeProxyURL 标准化代理 URL
// 处理空值和解析错误，返回标准化的键和解析后的 URL
//
// 参数:
//   - raw: 原始代理 URL 字符串
//
// 返回:
//   - string: 标准化的代理键（空返回 "direct"）
//   - *url.URL: 解析后的 URL（空返回 nil）
//   - error: 非空代理 URL 解析失败时返回错误（禁止回退到直连）
func normalizeProxyURL(raw string) (string, *url.URL, error) {
	_, parsed, err := proxyurl.Parse(raw)
	if err != nil {
		return "", nil, err
REDACTED
	if parsed == nil {
		return directProxyKey, nil, nil
REDACTED
	// 规范化：小写 scheme/host，去除路径和查询参数
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.ForceQuery = false
	if hostname := parsed.Hostname(); hostname != "" {
		port := parsed.Port()
		if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
			port = ""
	REDACTED
		hostname = strings.ToLower(hostname)
		if port != "" {
			parsed.Host = net.JoinHostPort(hostname, port)
	REDACTED else {
			parsed.Host = hostname
	REDACTED
REDACTED
	return parsed.String(), parsed, nil
REDACTED

// defaultPoolSettings 获取默认连接池配置
// 从全局配置中读取，无效值使用常量默认值
//
// 参数:
//   - cfg: 全局配置
//
// 返回:
//   - poolSettings: 连接池配置
func defaultPoolSettings(cfg *config.Config) poolSettings {
	maxIdleConns := defaultMaxIdleConns
	maxIdleConnsPerHost := defaultMaxIdleConnsPerHost
	maxConnsPerHost := defaultMaxConnsPerHost
	idleConnTimeout := defaultIdleConnTimeout
	responseHeaderTimeout := defaultResponseHeaderTimeout

	if cfg != nil {
		if cfg.Gateway.MaxIdleConns > 0 {
			maxIdleConns = cfg.Gateway.MaxIdleConns
	REDACTED
		if cfg.Gateway.MaxIdleConnsPerHost > 0 {
			maxIdleConnsPerHost = cfg.Gateway.MaxIdleConnsPerHost
	REDACTED
		if cfg.Gateway.MaxConnsPerHost >= 0 {
			maxConnsPerHost = cfg.Gateway.MaxConnsPerHost
	REDACTED
		if cfg.Gateway.IdleConnTimeoutSeconds > 0 {
			idleConnTimeout = time.Duration(cfg.Gateway.IdleConnTimeoutSeconds) * time.Second
	REDACTED
		if cfg.Gateway.ResponseHeaderTimeout >= 0 {
			responseHeaderTimeout = time.Duration(cfg.Gateway.ResponseHeaderTimeout) * time.Second
	REDACTED
REDACTED

	return poolSettings{
		maxIdleConns:          maxIdleConns,
		maxIdleConnsPerHost:   maxIdleConnsPerHost,
		maxConnsPerHost:       maxConnsPerHost,
		idleConnTimeout:       idleConnTimeout,
		responseHeaderTimeout: responseHeaderTimeout,
REDACTED
REDACTED

// buildUpstreamTransport 构建上游请求的 Transport
// 使用配置文件中的连接池参数，支持生产环境调优
//
// 参数:
//   - settings: 连接池配置
//   - proxyURL: 代理 URL（nil 表示直连）
//
// 返回:
//   - *http.Transport: 配置好的 Transport 实例
//   - error: 代理配置错误
//
// Transport 参数说明:
//   - MaxIdleConns: 所有主机的最大空闲连接总数
//   - MaxIdleConnsPerHost: 每主机最大空闲连接数（影响连接复用率）
//   - MaxConnsPerHost: 每主机最大连接数（达到后新请求等待）
//   - IdleConnTimeout: 空闲连接超时（超时后关闭）
//   - ResponseHeaderTimeout: 等待响应头超时（不影响流式传输）
func buildUpstreamTransport(settings poolSettings, proxyURL *url.URL, protocolMode string) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConns:          settings.maxIdleConns,
		MaxIdleConnsPerHost:   settings.maxIdleConnsPerHost,
		MaxConnsPerHost:       settings.maxConnsPerHost,
		IdleConnTimeout:       settings.idleConnTimeout,
		ResponseHeaderTimeout: settings.responseHeaderTimeout,
REDACTED
	switch protocolMode {
	case upstreamProtocolModeOpenAIH2:
		transport.ForceAttemptHTTP2 = true
	case upstreamProtocolModeOpenAIH1:
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	case upstreamProtocolModeOpenAIH1Fallback:
		// 显式禁用 HTTP/2，确保代理不兼容场景回退到 HTTP/1.1。
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
REDACTED
	if err := proxyutil.ConfigureTransportProxy(transport, proxyURL); err != nil {
		return nil, err
REDACTED
	return transport, nil
REDACTED

// buildUpstreamTransportWithTLSFingerprint 构建带 TLS 指纹伪装的 Transport
// 使用 utls 库模拟 Claude CLI 的 TLS 指纹
//
// 参数:
//   - settings: 连接池配置
//   - proxyURL: 代理 URL（nil 表示直连）
//   - profile: TLS 指纹配置
//
// 返回:
//   - *http.Transport: 配置好的 Transport 实例
//   - error: 配置错误
//
// 代理类型处理:
//   - nil/空: 直连，使用 TLSFingerprintDialer
//   - http/https: HTTP 代理，使用 HTTPProxyDialer（CONNECT 隧道 + utls 握手）
//   - socks5: SOCKS5 代理，使用 SOCKS5ProxyDialer（SOCKS5 隧道 + utls 握手）
func buildUpstreamTransportWithTLSFingerprint(settings poolSettings, proxyURL *url.URL, profile *tlsfingerprint.Profile) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConns:          settings.maxIdleConns,
		MaxIdleConnsPerHost:   settings.maxIdleConnsPerHost,
		MaxConnsPerHost:       settings.maxConnsPerHost,
		IdleConnTimeout:       settings.idleConnTimeout,
		ResponseHeaderTimeout: settings.responseHeaderTimeout,
		// 禁用默认的 TLS，我们使用自定义的 DialTLSContext
		ForceAttemptHTTP2: false,
REDACTED

	// 根据代理类型选择合适的 TLS 指纹 Dialer
	if proxyURL == nil {
		// 直连：使用 TLSFingerprintDialer
		slog.Debug("tls_fingerprint_transport_direct")
		dialer := tlsfingerprint.NewDialer(profile, nil)
		transport.DialTLSContext = dialer.DialTLSContext
REDACTED else {
		scheme := strings.ToLower(proxyURL.Scheme)
		switch scheme {
		case "socks5", "socks5h":
			// SOCKS5 代理：使用 SOCKS5ProxyDialer
			slog.Debug("tls_fingerprint_transport_socks5", "proxy", proxyURL.Host)
			socks5Dialer := tlsfingerprint.NewSOCKS5ProxyDialer(profile, proxyURL)
			transport.DialTLSContext = socks5Dialer.DialTLSContext
		case "http", "https":
			// HTTP/HTTPS 代理：使用 HTTPProxyDialer（CONNECT 隧道）
			slog.Debug("tls_fingerprint_transport_http_connect", "proxy", proxyURL.Host)
			httpDialer := tlsfingerprint.NewHTTPProxyDialer(profile, proxyURL)
			transport.DialTLSContext = httpDialer.DialTLSContext
		default:
			// 未知代理类型，回退到普通代理配置（无 TLS 指纹）
			slog.Debug("tls_fingerprint_transport_unknown_scheme_fallback", "scheme", scheme)
			if err := proxyutil.ConfigureTransportProxy(transport, proxyURL); err != nil {
				return nil, err
		REDACTED
	REDACTED
REDACTED

	return transport, nil
REDACTED

// trackedBody 带跟踪功能的响应体包装器
// 在 Close 时执行回调，用于更新请求计数
type trackedBody struct {
	io.ReadCloser // 原始响应体
	once          sync.Once
	onClose       func() // 关闭时的回调函数
REDACTED

// Close 关闭响应体并执行回调
// 使用 sync.Once 确保回调只执行一次
func (b *trackedBody) Close() error {
	err := b.ReadCloser.Close()
	if b.onClose != nil {
		b.once.Do(b.onClose)
REDACTED
	return err
REDACTED

// wrapTrackedBody 包装响应体以跟踪关闭事件
// 用于在响应体关闭时更新 inFlight 计数
//
// 参数:
//   - body: 原始响应体
//   - onClose: 关闭时的回调函数
//
// 返回:
//   - io.ReadCloser: 包装后的响应体
func wrapTrackedBody(body io.ReadCloser, onClose func()) io.ReadCloser {
	if body == nil {
		return body
REDACTED
	return &trackedBody{ReadCloser: body, onClose: onCloseREDACTED
REDACTED

// decompressResponseBody 根据 Content-Encoding 解压响应体。
// 当请求显式设置了 accept-encoding 时，Go 的 Transport 不会自动解压，需要手动处理。
// 解压成功后会删除 Content-Encoding 和 Content-Length header（长度已不准确）。
func decompressResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
REDACTED
	ce := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if ce == "" {
		return
REDACTED

	originalBody := resp.Body
	var reader io.Reader
	switch ce {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return // 解压失败，保持原样
	REDACTED
		reader = gr
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = flate.NewReader(resp.Body)
	case "zstd":
		bufferedBody := bufio.NewReader(resp.Body)
		resp.Body = &decompressedBody{reader: bufferedBody, closer: originalBodyREDACTED

		headerBytes, _ := bufferedBody.Peek(zstd.HeaderMaxSize)
		var header zstd.Header
		if err := header.Decode(headerBytes); err != nil {
			slog.Warn("zstd_decompress_failed", "error", err)
			return
	REDACTED

		zr, err := zstd.NewReader(bufferedBody)
		if err != nil {
			slog.Warn("zstd_decompress_failed", "error", err)
			return
	REDACTED
		reader = &zstdResponseReader{ReadCloser: zr.IOReadCloser()REDACTED
	default:
		return
REDACTED

	resp.Body = &decompressedBody{reader: reader, closer: originalBodyREDACTED
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length") // 解压后长度不确定
	resp.ContentLength = -1
REDACTED

type zstdResponseReader struct {
	io.ReadCloser
	warnOnce sync.Once
REDACTED

func (r *zstdResponseReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.warnOnce.Do(func() {
			slog.Warn("zstd_decompress_failed", "error", err)
	REDACTED)
REDACTED
	return n, err
REDACTED

// decompressedBody 组合解压 reader 和原始 body 的 close。
type decompressedBody struct {
	reader io.Reader
	closer io.Closer
REDACTED

func (d *decompressedBody) Read(p []byte) (int, error) {
	return d.reader.Read(p)
REDACTED

func (d *decompressedBody) Close() error {
	// 如果 reader 本身也是 Closer（如 gzip.Reader），先关闭它
	if rc, ok := d.reader.(io.Closer); ok {
		_ = rc.Close()
REDACTED
	return d.closer.Close()
REDACTED
