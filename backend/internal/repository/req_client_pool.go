package repository

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
)

// reqClientOptions 定义 req 客户端的构建参数
type reqClientOptions struct {
	ProxyURL    string        // 代理 URL（支持 http/https/socks5）
	Timeout     time.Duration // 请求超时时间
	Impersonate bool          // 是否模拟 Chrome 浏览器指纹
REDACTED

// sharedReqClients 存储按配置参数缓存的 req 客户端实例
//
// 性能优化说明：
// 原实现在每次 OAuth 刷新时都创建新的 req.Client：
// 1. claude_oauth_service.go: 每次刷新创建新客户端
// 2. openai_oauth_service.go: 每次刷新创建新客户端
// 3. gemini_oauth_client.go: 每次刷新创建新客户端
//
// 新实现使用 sync.Map 缓存客户端：
// 1. 相同配置（代理+超时+模拟设置）复用同一客户端
// 2. 复用底层连接池，减少 TLS 握手开销
// 3. LoadOrStore 保证并发安全，避免重复创建
var sharedReqClients sync.Map

// getSharedReqClient 获取共享的 req 客户端实例
// 性能优化：相同配置复用同一客户端，避免重复创建
func getSharedReqClient(opts reqClientOptions) *req.Client {
	key := buildReqClientKey(opts)
	if cached, ok := sharedReqClients.Load(key); ok {
		if c, ok := cached.(*req.Client); ok {
			return c
	REDACTED
REDACTED

	client := req.C().SetTimeout(opts.Timeout)
	if opts.Impersonate {
		client = client.ImpersonateChrome()
REDACTED
	if strings.TrimSpace(opts.ProxyURL) != "" {
		client.SetProxyURL(strings.TrimSpace(opts.ProxyURL))
REDACTED

	actual, _ := sharedReqClients.LoadOrStore(key, client)
	if c, ok := actual.(*req.Client); ok {
		return c
REDACTED
	return client
REDACTED

func buildReqClientKey(opts reqClientOptions) string {
	return fmt.Sprintf("%s|%s|%t",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.Impersonate,
	)
REDACTED
