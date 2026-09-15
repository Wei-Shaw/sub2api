// devin_gateway_service.go 把 Devin 平台账号接到 Connect-RPC 上游：
// 每个账号一个 adapter（绑定代理/TLS 指纹/并发计数），对外暴露
// llm.ResponseStream 与分组模型目录。账号选择/计费/熔断由既有
// GatewayService 与 handler 层负责，本服务只做传输适配。
package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	devinadapter "github.com/Wei-Shaw/sub2api/internal/pkg/devin/adapter"
	devincommon "github.com/Wei-Shaw/sub2api/internal/pkg/devin/api/common"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// devinAdapterIdleTTL 是 adapter 空闲回收窗口：超过即重建。adapter 自身
// 对 catalog/AssignModel 的缓存有正确性保障，空闲重建只是丢掉缓存热度。
const devinAdapterIdleTTL = 30 * time.Minute

// devinAdapterCacheCap 是 adapter 缓存的硬上限（最久未用逐出）。
const devinAdapterCacheCap = 512

// DevinGatewayService 为 Devin 账号构造并驱动 Connect adapter。
type DevinGatewayService struct {
	httpUpstream       HTTPUpstream
	tlsFingerprintProf *TLSFingerprintProfileService

	// adapters 按账号指纹缓存复用 adapter：catalog（5min TTL）与
	// AssignModel 解析两级缓存都挂在 adapter 实例上，每请求新建会让
	// 两级缓存全部失效——每条流前多出 GetCliModelConfigs + AssignModel
	// 两趟上游 RPC，直接体现为首字延迟。
	adaptersMu sync.Mutex
	adapters   map[string]cachedDevinAdapter
}

type cachedDevinAdapter struct {
	adapter    *devinadapter.Adapter
	lastUsedAt time.Time
}

// NewDevinGatewayService 创建服务。
func NewDevinGatewayService(httpUpstream HTTPUpstream, tlsFingerprintProf *TLSFingerprintProfileService) *DevinGatewayService {
	return &DevinGatewayService{
		httpUpstream:       httpUpstream,
		tlsFingerprintProf: tlsFingerprintProf,
		adapters:           make(map[string]cachedDevinAdapter),
	}
}

// adapterForAccount 把账号凭据/代理/TLS 指纹绑定为一个 adapter。
// 命中缓存直接复用；凭据/地址/代理/并发任一变化都会改变指纹而换用新 adapter。
func (s *DevinGatewayService) adapterForAccount(account *Account) (*devinadapter.Adapter, error) {
	if account == nil || !account.IsDevin() {
		return nil, errors.New("account is not a devin platform account")
	}
	token := account.GetDevinToken()
	if token == "" {
		return nil, errors.New("devin account has no access_token credential")
	}
	baseURL := account.GetDevinBaseURL()
	if baseURL == "" {
		baseURL = devin.DefaultBaseURL
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	profileDoer := func(req *http.Request) (*http.Response, error) {
		profile := s.tlsFingerprintProf.ResolveTLSProfile(account)
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, profile)
	}
	// 指纹含 token 哈希（不落明文）/base_url/client_version/代理/并发，
	// 任何凭据或传输配置漂移都自动失效换新。
	tokenHash := sha256.Sum256([]byte(token))
	key := fmt.Sprintf("%d|%x|%s|%s|%s|%d", account.ID, tokenHash[:8],
		baseURL, account.GetDevinClientVersion(), proxyURL, account.Concurrency)
	now := time.Now()

	s.adaptersMu.Lock()
	defer s.adaptersMu.Unlock()
	if entry, ok := s.adapters[key]; ok && now.Sub(entry.lastUsedAt) < devinAdapterIdleTTL {
		entry.lastUsedAt = now
		s.adapters[key] = entry
		return entry.adapter, nil
	}
	ad, err := devinadapter.New(devinadapter.Config{
		BaseURL:       baseURL,
		Token:         token,
		ClientVersion: account.GetDevinClientVersion(),
		Do:            profileDoer,
	})
	if err != nil {
		return nil, err
	}
	// 顺手清扫过期项；超限则逐出最久未用的。
	for k, entry := range s.adapters {
		if now.Sub(entry.lastUsedAt) >= devinAdapterIdleTTL {
			delete(s.adapters, k)
		}
	}
	for len(s.adapters) >= devinAdapterCacheCap {
		var oldestKey string
		var oldestAt time.Time
		for k, entry := range s.adapters {
			if oldestKey == "" || entry.lastUsedAt.Before(oldestAt) {
				oldestKey, oldestAt = k, entry.lastUsedAt
			}
		}
		delete(s.adapters, oldestKey)
	}
	s.adapters[key] = cachedDevinAdapter{adapter: ad, lastUsedAt: now}
	return ad, nil
}

// Stream 打开一条 GetChatMessage 流，返回供应商无关的增量事件流。
func (s *DevinGatewayService) Stream(ctx context.Context, account *Account, request llm.RequestMessages) (llm.ResponseStream, error) {
	ad, err := s.adapterForAccount(account)
	if err != nil {
		return nil, err
	}
	return ad.Stream(ctx, request)
}

// ListModels 返回账号的分组模型目录（带 adapter 内 TTL 缓存）。
func (s *DevinGatewayService) ListModels(ctx context.Context, account *Account) ([]devin.GroupedModel, error) {
	ad, err := s.adapterForAccount(account)
	if err != nil {
		return nil, err
	}
	return ad.ListModels(ctx)
}

// DevinFailoverError 把 adapter/上游错误归类为「换账号重试」或「客户端错误」。
// 语义与 Connect code 对应：
//   - unauthenticated → 凭据失败（换号，并由 rateLimitService 标记账号）
//   - resource_exhausted / unavailable / deadline / 传输瞬断 → 换号重试
//   - invalid_argument / failed_precondition / context 超长 → 客户端 4xx，不换号
func DevinFailoverError(err error) *UpstreamFailoverError {
	if err == nil {
		return nil
	}
	var connectErr *devin.ConnectError
	msg := err.Error()
	if errors.As(err, &connectErr) {
		msg = strings.TrimSpace(connectErr.Code + ": " + connectErr.Message)
		status := connectErr.HTTPStatus
		if status == 0 {
			status = devinStatusForCode(connectErr.Code)
		}
		failover := &UpstreamFailoverError{
			StatusCode:    status,
			ResponseBody:  []byte(msg),
			ClientMessage: msg,
		}
		switch {
		case devin.IsUnauthenticated(connectErr):
			failover.Stage = GatewayFailureStageAccountAuth
			failover.ClientStatusCode = http.StatusUnauthorized
		case devin.IsCode(connectErr, "resource_exhausted"):
			failover.ClientStatusCode = http.StatusTooManyRequests
		case devin.IsCode(connectErr, "invalid_argument"), devin.IsCode(connectErr, "failed_precondition"):
			// 请求形状错误——换号无意义，直返客户端。
			failover.NextAccountAction = NextAccountStop
			failover.ClientStatusCode = http.StatusBadRequest
			if devincommon.IsContextLengthError(msg) {
				failover.ClientStatusCode = http.StatusRequestEntityTooLarge
			}
		case devin.IsCode(connectErr, "permission_denied"):
			failover.NextAccountAction = NextAccountStop
			failover.ClientStatusCode = http.StatusForbidden
		default:
			// unavailable/internal/deadline_exceeded 等 → 换号重试
			failover.ClientStatusCode = http.StatusBadGateway
		}
		return failover
	}
	if devin.IsTransientTransportError(err) {
		return &UpstreamFailoverError{
			StatusCode:       http.StatusBadGateway,
			ResponseBody:     []byte(msg),
			ClientMessage:    msg,
			ClientStatusCode: http.StatusBadGateway,
		}
	}
	// 本地校验/编码错误（tool_choice、图片越界等）→ 客户端错误。
	// IsInvalidRequest 命中的显式包装与未分类错误都走这条路径：
	// 未识别的本地错误宁可直返 400 也不在账号间空转。
	return &UpstreamFailoverError{
		StatusCode:        http.StatusBadRequest,
		ResponseBody:      []byte(msg),
		ClientMessage:     msg,
		ClientStatusCode:  http.StatusBadRequest,
		NextAccountAction: NextAccountStop,
	}
}

// devinStatusForCode 在无 HTTP 状态（end-trailer 错误）时按 Connect code
// 推断用于账号熔断的状态码。
func devinStatusForCode(code string) int {
	switch code {
	case "unauthenticated":
		return http.StatusUnauthorized
	case "permission_denied":
		return http.StatusForbidden
	case "resource_exhausted":
		return http.StatusTooManyRequests
	case "invalid_argument", "failed_precondition", "out_of_range":
		return http.StatusBadRequest
	case "not_found":
		return http.StatusNotFound
	case "unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// DevinRequestID 提取上游 request_id（若有）便于报障。
func DevinRequestID(msg *llm.AssistantMessage) string {
	if msg == nil {
		return ""
	}
	return msg.UpstreamRequestID
}
