package repository

import (
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

// upstreamClientEntry 上游客户端缓存条目
// 记录客户端实例及其元数据，用于连接池管理和淘汰策略
type upstreamClientEntry struct {
	client   *http.Client // HTTP 客户端实例
	proxyKey string       // 代理标识（用于检测代理变更）
	poolKey  string       // 连接池配置标识（用于检测配置变更）
	lastUsed int64        // 最后使用时间戳（纳秒），用于 LRU 淘汰
	inFlight int64        // 当前进行中的请求数，>0 时不可淘汰
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

	// 获取或创建对应的客户端，并标记请求占用
	entry, err := s.acquireClient(proxyURL, accountID, accountConcurrency)
	if err != nil {
		return nil, err
REDACTED

	// 执行请求
	resp, err := entry.client.Do(req)
	if err != nil {
		// 请求失败，立即减少计数
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
		return nil, err
REDACTED

	// 包装响应体，在关闭时自动减少计数并更新时间戳
	// 这确保了流式响应（如 SSE）在完全读取前不会被淘汰
	resp.Body = wrapTrackedBody(resp.Body, func() {
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
REDACTED)

	return resp, nil
REDACTED

// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
// 根据 enableTLSFingerprint 参数决定是否使用 TLS 指纹
//
// 参数:
//   - req: HTTP 请求对象
//   - proxyURL: 代理地址，空字符串表示直连
//   - accountID: 账户 ID，用于账户级隔离和 TLS 指纹模板选择
//   - accountConcurrency: 账户并发限制，用于动态调整连接池大小
//   - enableTLSFingerprint: 是否启用 TLS 指纹伪装
//
// TLS 指纹说明:
//   - 当 enableTLSFingerprint=true 时，使用 utls 库模拟 Claude CLI 的 TLS 指纹
//   - 指纹模板根据 accountID % len(profiles) 自动选择
//   - 支持直连、HTTP/HTTPS 代理、SOCKS5 代理三种场景
func (s *httpUpstreamService) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error) {
	// 如果未启用 TLS 指纹，直接使用标准请求路径
	if !enableTLSFingerprint {
		return s.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

	// TLS 指纹已启用，记录调试日志
	targetHost := ""
	if req != nil && req.URL != nil {
		targetHost = req.URL.Host
REDACTED
	proxyInfo := "direct"
	if proxyURL != "" {
		proxyInfo = proxyURL
REDACTED
	slog.Debug("tls_fingerprint_enabled", "account_id", accountID, "target", targetHost, "proxy", proxyInfo)

	if err := s.validateRequestHost(req); err != nil {
		return nil, err
REDACTED

	// 获取 TLS 指纹 Profile
	registry := tlsfingerprint.GlobalRegistry()
	profile := registry.GetProfileByAccountID(accountID)
	if profile == nil {
		// 如果获取不到 profile，回退到普通请求
		slog.Debug("tls_fingerprint_no_profile", "account_id", accountID, "fallback", "standard_request")
		return s.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

	slog.Debug("tls_fingerprint_using_profile", "account_id", accountID, "profile", profile.Name, "grease", profile.EnableGREASE)

	// 获取或创建带 TLS 指纹的客户端
	entry, err := s.acquireClientWithTLS(proxyURL, accountID, accountConcurrency, profile)
	if err != nil {
		slog.Debug("tls_fingerprint_acquire_client_failed", "account_id", accountID, "error", err)
		return nil, err
REDACTED

	// 执行请求
	resp, err := entry.client.Do(req)
	if err != nil {
		// 请求失败，立即减少计数
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
		slog.Debug("tls_fingerprint_request_failed", "account_id", accountID, "error", err)
		return nil, err
REDACTED

	slog.Debug("tls_fingerprint_request_success", "account_id", accountID, "status", resp.StatusCode)

	// 包装响应体，在关闭时自动减少计数并更新时间戳
	resp.Body = wrapTrackedBody(resp.Body, func() {
		atomic.AddInt64(&entry.inFlight, -1)
		atomic.StoreInt64(&entry.lastUsed, time.Now().UnixNano())
REDACTED)

	return resp, nil
REDACTED

// acquireClientWithTLS 获取或创建带 TLS 指纹的客户端
func (s *httpUpstreamService) acquireClientWithTLS(proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*upstreamClientEntry, error) {
	return s.getClientEntryWithTLS(proxyURL, accountID, accountConcurrency, profile, true, true)
REDACTED

// getClientEntryWithTLS 获取或创建带 TLS 指纹的客户端条目
// TLS 指纹客户端使用独立的缓存键，与普通客户端隔离
func (s *httpUpstreamService) getClientEntryWithTLS(proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile, markInFlight bool, enforceLimit bool) (*upstreamClientEntry, error) {
	isolation := s.getIsolationMode()
	proxyKey, parsedProxy, err := normalizeProxyURL(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	// TLS 指纹客户端使用独立的缓存键，加 "tls:" 前缀
	cacheKey := "tls:" + buildCacheKey(isolation, proxyKey, accountID)
	poolKey := s.buildPoolKey(isolation, accountConcurrency) + ":tls"

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
	settings := s.resolvePoolSettings(isolation, accountConcurrency)
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
	return s.getClientEntry(proxyURL, accountID, accountConcurrency, true, true)
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
	return s.getClientEntry(proxyURL, accountID, accountConcurrency, false, false)
REDACTED

// getClientEntry 获取或创建客户端条目
// markInFlight=true 时会标记进行中请求，用于请求路径防止被淘汰
// enforceLimit=true 时会限制客户端数量，超限且无法淘汰时返回错误
func (s *httpUpstreamService) getClientEntry(proxyURL string, accountID int64, accountConcurrency int, markInFlight bool, enforceLimit bool) (*upstreamClientEntry, error) {
	// 获取隔离模式
	isolation := s.getIsolationMode()
	// 标准化代理 URL 并解析
	proxyKey, parsedProxy, err := normalizeProxyURL(proxyURL)
	if err != nil {
		return nil, err
REDACTED
	// 构建缓存键（根据隔离策略不同）
	cacheKey := buildCacheKey(isolation, proxyKey, accountID)
	// 构建连接池配置键（用于检测配置变更）
	poolKey := s.buildPoolKey(isolation, accountConcurrency)

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
	settings := s.resolvePoolSettings(isolation, accountConcurrency)
	transport, err := buildUpstreamTransport(settings, parsedProxy)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("build transport: %w", err)
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

// buildPoolKey 构建连接池配置键
// 用于检测配置变更，配置变更时需要重建客户端
//
// 参数:
//   - isolation: 隔离模式
//   - accountConcurrency: 账户并发限制
//
// 返回:
//   - string: 配置键
func (s *httpUpstreamService) buildPoolKey(isolation string, accountConcurrency int) string {
	if isolation == config.ConnectionPoolIsolationAccount || isolation == config.ConnectionPoolIsolationAccountProxy {
		if accountConcurrency > 0 {
			return fmt.Sprintf("account:%d", accountConcurrency)
	REDACTED
REDACTED
	return "default"
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
func buildCacheKey(isolation, proxyKey string, accountID int64) string {
	switch isolation {
	case config.ConnectionPoolIsolationAccount:
		return fmt.Sprintf("account:%d", accountID)
	case config.ConnectionPoolIsolationAccountProxy:
		return fmt.Sprintf("account:%d|proxy:%s", accountID, proxyKey)
	default:
		return fmt.Sprintf("proxy:%s", proxyKey)
REDACTED
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
		if cfg.Gateway.ResponseHeaderTimeout > 0 {
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
func buildUpstreamTransport(settings poolSettings, proxyURL *url.URL) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConns:          settings.maxIdleConns,
		MaxIdleConnsPerHost:   settings.maxIdleConnsPerHost,
		MaxConnsPerHost:       settings.maxConnsPerHost,
		IdleConnTimeout:       settings.idleConnTimeout,
		ResponseHeaderTimeout: settings.responseHeaderTimeout,
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
