package service

import "net/http"

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
// 这是一个通用接口，可用于任何基于 HTTP 的上游服务
//
// 设计说明：
// - 支持可选代理配置
// - 支持账户级连接池隔离
// - 实现类负责连接池管理和复用
// - 支持可选的 TLS 指纹伪装
type HTTPUpstream interface {
	// Do 执行 HTTP 请求
	//
	// 参数:
	//   - req: HTTP 请求对象，由调用方构建
	//   - proxyURL: 代理服务器地址，空字符串表示直连
	//   - accountID: 账户 ID，用于连接池隔离（隔离策略为 account 或 account_proxy 时生效）
	//   - accountConcurrency: 账户并发限制，用于动态调整连接池大小
	//
	// 返回:
	//   - *http.Response: HTTP 响应，调用方必须关闭 Body
	//   - error: 请求错误（网络错误、超时等）
	//
	// 注意:
	//   - 调用方必须关闭 resp.Body，否则会导致连接泄漏
	//   - 响应体可能已被包装以跟踪请求生命周期
	Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)

	// DoWithTLS 执行带 TLS 指纹伪装的 HTTP 请求
	//
	// 参数:
	//   - req: HTTP 请求对象，由调用方构建
	//   - proxyURL: 代理服务器地址，空字符串表示直连
	//   - accountID: 账户 ID，用于连接池隔离和 TLS 指纹模板选择
	//   - accountConcurrency: 账户并发限制，用于动态调整连接池大小
	//   - enableTLSFingerprint: 是否启用 TLS 指纹伪装
	//
	// 返回:
	//   - *http.Response: HTTP 响应，调用方必须关闭 Body
	//   - error: 请求错误（网络错误、超时等）
	//
	// TLS 指纹说明:
	//   - 当 enableTLSFingerprint=true 时，使用 utls 库模拟 Claude CLI 的 TLS 指纹
	//   - TLS 指纹模板根据 accountID % len(profiles) 自动选择
	//   - 支持直连、HTTP/HTTPS 代理、SOCKS5 代理三种场景
	//   - 如果 enableTLSFingerprint=false，行为与 Do 方法相同
	//
	// 注意:
	//   - 调用方必须关闭 resp.Body，否则会导致连接泄漏
	//   - TLS 指纹客户端与普通客户端使用不同的缓存键，互不影响
	DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error)
REDACTED
