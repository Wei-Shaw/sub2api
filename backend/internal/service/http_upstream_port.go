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
REDACTED
