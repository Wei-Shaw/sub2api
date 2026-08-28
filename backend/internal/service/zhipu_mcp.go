package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// zhipuMCPBillableMethod 唯一计费的 JSON-RPC 方法：tools/call 是唯一会触发
// 上游真实工作（检索/抓取等）的 MCP 方法；initialize / tools/list / 其余
// notifications/* 均为管理性或通知性流量，免费放行。
const zhipuMCPBillableMethod = "tools/call"

// DefaultZhipuMCPBaseURL 智谱远程 MCP Server 的默认基地址。
// Coding Plan 订阅附带的 Streamable HTTP MCP 端点形如 {base}/{slug}/mcp。
const DefaultZhipuMCPBaseURL = "https://open.bigmodel.cn/api/mcp"

// ZhipuMCPSessionTTL 智谱 MCP session 粘表条目的存活时间。
// 上游未公开 session 的真实有效期（官方文档未标注，且不同 slug 可能不同），
// 30 分钟是保守值：过期后客户端再带同一 Mcp-Session-Id 请求会走正常调度重新选号，
// 若上游侧 session 仍在，只是换了出口账号，行为等价于"粘性失效"，不会造成协议错误。
const ZhipuMCPSessionTTL = 30 * time.Minute

// ErrZhipuMCPSessionNotFound 粘表中不存在该 MCP session 绑定。
// 与 ErrStickySessionNotFound 同语义：让调用方区分"未绑定"与真实读取失败。
var ErrZhipuMCPSessionNotFound = errors.New("zhipu mcp session not found")

// zhipuMCPServerSlugs 已实测可用的智谱远程 MCP Server slug 白名单。
// web_reader / vision 等其余 slug 未实测，暂不放进第一期表；实测确认后再补充。
var zhipuMCPServerSlugs = map[string]struct{}{
	"web_search_prime": {},
	"zread":            {},
}

// ResolveZhipuMCPServerURL 把 MCP slug 解析为智谱上游 Streamable HTTP 端点。
// 未在白名单内的 slug 返回 false，由调用方映射为 404，避免把任意路径透传到上游。
func ResolveZhipuMCPServerURL(slug string) (string, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", false
	}
	if _, ok := zhipuMCPServerSlugs[slug]; !ok {
		return "", false
	}
	return DefaultZhipuMCPBaseURL + "/" + slug + "/mcp", true
}

// CountZhipuMCPBillableCalls 统计一次 MCP 请求中的可计费 tools/call 数量
// （Phase 2B 计费口径）：
//   - body 为 JSON 对象：method == "tools/call" 且带非 null id（JSON-RPC Request）
//     计 1；其余方法（initialize / tools/list 等）与缺 id 的通知（JSON-RPC
//     Notification 语义，id 缺失即通知）计 0。
//   - body 为 JSON 数组（JSON-RPC batch）：按同样规则逐元素计数。
//   - 解析失败 / 非对象非数组（空 body、标量、null 等）一律返回 0：宁可漏计
//     不可误计，畸形请求不应产生用户费用。
//
// 不校验 jsonrpc 版本字段：计费发生在上游已受理（HTTP < 400）之后，method + id
// 判定足以区分"真实调用"与"管理/通知流量"。
func CountZhipuMCPBillableCalls(body []byte) int {
	parsed := gjson.ParseBytes(body)
	if parsed.IsObject() {
		return countZhipuMCPBillableCallResult(parsed)
	}
	if parsed.IsArray() {
		count := 0
		parsed.ForEach(func(_, item gjson.Result) bool {
			count += countZhipuMCPBillableCallResult(item)
			return true
		})
		return count
	}
	return 0
}

// countZhipuMCPBillableCallResult 按计费口径判定单个 JSON-RPC 消息：
// 仅"带 id 的 tools/call 请求"计 1，其余（通知、其它方法、非对象元素）计 0。
func countZhipuMCPBillableCallResult(item gjson.Result) int {
	if !item.IsObject() || item.Get("method").String() != zhipuMCPBillableMethod {
		return 0
	}
	// id 缺失或显式 null → JSON-RPC Notification 语义，不计费。
	if id := item.Get("id"); !id.Exists() || id.Type == gjson.Null {
		return 0
	}
	return 1
}

// ZhipuMCPSessionStore 智谱 MCP session 粘表存储（SETEX 语义）。
// 与 GatewayCache 的粘性会话接口分开：MCP session 以客户端回传的 Mcp-Session-Id 为键，
// 不参与模型转发的 sticky session 生命周期（清理时机、TTL、命名空间都不同）。
type ZhipuMCPSessionStore interface {
	// SetZhipuMCPSession 绑定 MCP session 与账号，ttl 到期自动失效。
	SetZhipuMCPSession(ctx context.Context, sessionID string, accountID int64, ttl time.Duration) error
	// GetZhipuMCPSession 读取绑定；未命中返回 ErrZhipuMCPSessionNotFound。
	GetZhipuMCPSession(ctx context.Context, sessionID string) (int64, error)
	// DeleteZhipuMCPSession 删除绑定（客户端 DELETE 终止 session 或粘表失效时调用）。
	DeleteZhipuMCPSession(ctx context.Context, sessionID string) error
}

// sanitizeZhipuMCPSessionID 校验客户端回传的 Mcp-Session-Id。
// 该值会拼进 Redis key（防 key 注入）并写入日志（防日志注入/刷屏），
// 因此仅接受 1..200 长度、可见 ASCII（MCP session id 实际为 hex/base64 形态）。
func sanitizeZhipuMCPSessionID(sessionID string) (string, bool) {
	s := strings.TrimSpace(sessionID)
	if len(s) < 1 || len(s) > 200 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return "", false
		}
	}
	return s, true
}

// WithZhipuMCPSessionStore 注入可选的 MCP session 粘表存储。
// 保持 NewGatewayService 的测试构造签名不变（ProvideGatewayService 负责接线）；
// 未注入时粘表方法按 best-effort 降级，仅失去跨请求的账号亲和。
func (s *GatewayService) WithZhipuMCPSessionStore(store ZhipuMCPSessionStore) *GatewayService {
	if s == nil {
		return s
	}
	s.zhipuMCPSessions = store
	return s
}

// BindZhipuMCPSession 绑定 MCP session 与账号（SETEX）。
// upstream 只在 initialize 等场景下发 Mcp-Session-Id，调用方应仅在收到该响应头时绑定。
func (s *GatewayService) BindZhipuMCPSession(ctx context.Context, sessionID string, accountID int64) error {
	if s == nil || s.zhipuMCPSessions == nil || accountID <= 0 {
		return nil
	}
	id, ok := sanitizeZhipuMCPSessionID(sessionID)
	if !ok {
		return fmt.Errorf("invalid zhipu mcp session id")
	}
	return s.zhipuMCPSessions.SetZhipuMCPSession(ctx, id, accountID, ZhipuMCPSessionTTL)
}

// LookupZhipuMCPSession 读取 MCP session 绑定的账号；未绑定或存储异常统一返回 false，
// 调用方回退到正常调度（粘性是优化，不是正确性依赖）。
func (s *GatewayService) LookupZhipuMCPSession(ctx context.Context, sessionID string) (int64, bool) {
	if s == nil || s.zhipuMCPSessions == nil {
		return 0, false
	}
	id, ok := sanitizeZhipuMCPSessionID(sessionID)
	if !ok {
		return 0, false
	}
	accountID, err := s.zhipuMCPSessions.GetZhipuMCPSession(ctx, id)
	if err != nil || accountID <= 0 {
		return 0, false
	}
	return accountID, true
}

// UnbindZhipuMCPSession 删除 MCP session 绑定。删除失败不影响主流程：
// 粘表条目有 TTL 兜底，最坏情况是把请求继续粘到旧账号，由下游按 4xx 透传给客户端。
func (s *GatewayService) UnbindZhipuMCPSession(ctx context.Context, sessionID string) error {
	if s == nil || s.zhipuMCPSessions == nil {
		return nil
	}
	id, ok := sanitizeZhipuMCPSessionID(sessionID)
	if !ok {
		return nil
	}
	return s.zhipuMCPSessions.DeleteZhipuMCPSession(ctx, id)
}

// LoadZhipuMCPStickyAccount 加载并校验粘表指向的账号。
// 取数先例与 getSchedulableAccount 一致（调度快照优先，回退 accountRepo.GetByID，
// 软删除的账号在仓储层即查不到）；校验不通过返回错误，由调用方清粘表走正常调度。
func (s *GatewayService) LoadZhipuMCPStickyAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || accountID <= 0 {
		return nil, fmt.Errorf("invalid zhipu mcp sticky account id %d", accountID)
	}
	account, err := s.getSchedulableAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrNoAvailableAccounts
	}
	// 粘表写入时不校验能力开关之外的运行态，这里统一兜底：
	// 账号被改成非 zhipu / 关掉 MCP 开关 / 停用后，粘性必须立即失效而不是把请求打挂。
	if account.Platform != PlatformZhipu || !account.IsZhipuMCPCapable() || account.Status != StatusActive {
		return nil, fmt.Errorf("zhipu mcp sticky account %d not eligible", accountID)
	}
	return account, nil
}

// DoZhipuMCPUpstream 执行智谱 MCP 上游请求并返回原始响应，供 handler 透传（含 SSE 流式）。
// 上游客户端复用网关转发的 HTTPUpstream（连接池按账号隔离、支持代理）；
// 代理语义与 DoGrokNativeResponsesJSON 等先例一致：账号绑定代理时经代理出站。
// 调用方负责关闭返回的 resp.Body。
func (s *GatewayService) DoZhipuMCPUpstream(account *Account, upstreamReq *http.Request) (*http.Response, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("http upstream not configured")
	}
	if account == nil {
		return nil, errors.New("account is required")
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
}
