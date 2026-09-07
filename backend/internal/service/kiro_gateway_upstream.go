package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"golang.org/x/sync/singleflight"
)

// kiroUpstreamTimeout 是单次上游请求的总超时。
// 流式响应可能长时间保持，取值需覆盖最长的一次生成。
const kiroUpstreamTimeout = 10 * time.Minute

// KiroGatewayService 负责把 Anthropic 请求转发到 Kiro 上游。
//
// 结构对齐 AntigravityGatewayService：本文件只管「怎么把一次请求发出去」，
// 编排、流式写出与错误分类在 kiro_gateway_service.go。
//
// 账号选择/并发控制/计费落账不在本结构体的职责内——复用既有的 Messages
// 编排入口（Task 18 接线），这里只持有转发一次请求所需的最小依赖集：
// 账号仓储（重读账号 / 落库设备指纹）、OAuth 刷新入口、Kiro 刷新执行器、
// 运行时调度熔断器。不加限流/计费/配额依赖——那些是 Task 19-21 的范围。
type KiroGatewayService struct {
	// clientProfile 可被测试或配置覆盖。
	clientProfile *kiro.ClientProfile

	accountRepo      AccountRepository
	oauthRefreshAPI  *OAuthRefreshAPI
	kiroOAuthService *KiroOAuthService
	proxyRepo        ProxyRepository

	// schedulerSnapshot 用于 credits 耗尽时立即更新 Redis 里的账号调度快照
	// （model_rate_limits），避免调度层要等下一次全量同步才看到最新冷却状态。
	// 对齐 AntigravityGatewayService 同名字段的用法（antigravity_gateway_retry.go
	// 的 updateAccountModelRateLimitInCache）。可为 nil——写缓存失败时降级为
	// 只有 SetModelRateLimit 落库生效，调度快照会在下次全量同步时追上。
	schedulerSnapshot *SchedulerSnapshotService

	// creditsQuotaFlight 防止同一账号的并发失败请求各自独立发起
	// getUsageLimits 现场查询（惊群）——credits 真耗尽时往往是一批并发请求
	// 同时失败，N 个请求应该共享一次查询结果，而不是各打一次上游。
	// 零值即可用，见 creditsExhaustedCooldownUntil。对齐
	// account_usage_service.go 里 apiFlight/antigravityFlight 的既有用法。
	creditsQuotaFlight singleflight.Group

	// creditsQuotaFetcherOverride 仅供测试使用，生产路径必须为 nil。
	//
	// creditsExhaustedCooldownUntil 默认用 NewKiroQuotaFetcher() 现场构造一个
	// 指向真实 q.<region>.amazonaws.com 的 fetcher；测试需要把它指向本地
	// httptest 假上游才能验证 singleflight 去重效果，通过设置这个字段覆盖。
	creditsQuotaFetcherOverride *KiroQuotaFetcher

	// callEndpointOverride 仅供测试使用，生产路径必须为 nil。
	//
	// kiro.EndpointsFor 返回的是真实的 AWS/CLI 域名（q.<region>.amazonaws.com
	// 等），单元测试环境里既连不通也不该连——ForwardUpstream 的集成测试要
	// 验证的是编排逻辑（跨端点重试、decideKiroAction 的执行、流式转译），
	// 不是 Task 16 已经用 httptest 独立测过的请求头构造。测试通过设置这个
	// 字段，把请求路由到本地 httptest 假上游，同时保留 kiro.EndpointsFor
	// 真实的端点数量/顺序，让 hasMoreEndpoints 相关的决策分支照常被真实
	// 触达。见 forwardCallEndpoint。
	callEndpointOverride func(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error)

	// promptCache 是账号级、进程内的本地 prompt cache 模拟（kiro.PromptCacheTracker
	// 的文档有完整的问题背景与设计取舍）。零值（nil）时 forwardUpstream 的
	// Compute/Update 调用全部安全降级为 no-op——这就是为什么本文件其余全部
	// 既有测试（都用 &KiroGatewayService{...} 结构体字面量构造，不走
	// NewKiroGatewayService）在没有显式设置这个字段时行为完全不变：真实
	// meteringEvent 优先级不变，没有 meteringEvent 时也不会凭空冒出模拟值。
	promptCache *kiro.PromptCacheTracker
}

// creditsQuotaFetcher 返回 creditsExhaustedCooldownUntil 使用的额度获取器：
// 测试设置了 creditsQuotaFetcherOverride 时用它，否则用真实的
// NewKiroQuotaFetcher()。
func (s *KiroGatewayService) creditsQuotaFetcher() *KiroQuotaFetcher {
	if s.creditsQuotaFetcherOverride != nil {
		return s.creditsQuotaFetcherOverride
	}
	return NewKiroQuotaFetcher()
}

// forwardCallEndpoint 是 ForwardUpstream 实际使用的调用入口：测试设置了
// callEndpointOverride 时用它，否则用真实的 callEndpoint。
func (s *KiroGatewayService) forwardCallEndpoint(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error) {
	if s.callEndpointOverride != nil {
		return s.callEndpointOverride(ctx, account, ep, payload)
	}
	return s.callEndpoint(ctx, account, ep, payload)
}

// NewKiroGatewayService 创建转发编排服务。
//
// 不接 AccountRuntimeBlocker：它唯一的绑定实现 OpenAIGatewayService.
// BlockAccountScheduling 对 platform 做了 openai/grok 专属门禁
// （isOpenAIAccount），kiro 账号在这里恒为 no-op；kiro 自己的调度冷却走
// model_rate_limits 机制（model_rate_limit.go 的 PlatformKiro case），
// 已经足够且已验证生效——加一个永远读不到的 blocker 依赖只是误导性的
// 死代码（真实账号测试后走查代码发现，见 finishWithAction 的说明）。
func NewKiroGatewayService(
	accountRepo AccountRepository,
	oauthRefreshAPI *OAuthRefreshAPI,
	kiroOAuthService *KiroOAuthService,
	schedulerSnapshot *SchedulerSnapshotService,
	proxyRepo ProxyRepository,
) *KiroGatewayService {
	return &KiroGatewayService{
		accountRepo:       accountRepo,
		oauthRefreshAPI:   oauthRefreshAPI,
		kiroOAuthService:  kiroOAuthService,
		schedulerSnapshot: schedulerSnapshot,
		proxyRepo:         proxyRepo,
		promptCache:       kiro.NewPromptCacheTracker(),
	}
}

// profile 返回生效的客户端版本组合。
func (s *KiroGatewayService) profile() kiro.ClientProfile {
	if s != nil && s.clientProfile != nil {
		return *s.clientProfile
	}
	return kiro.DefaultClientProfile()
}

// callEndpoint 向指定端点发起一次请求。
//
// 调用方负责按 kiro.EndpointsFor 的顺序重试；本函数只发一次。
// 返回的 *http.Response 由调用方关闭。
func (s *KiroGatewayService) callEndpoint(ctx context.Context, account *Account, ep kiro.Endpoint, payload []byte) (*http.Response, error) {
	if account == nil {
		return nil, fmt.Errorf("kiro: account is required")
	}

	// 首次使用时固化设备指纹。返回 true 说明是新生成的，
	// 调用方（ForwardUpstream）负责把 credentials 落库——见该函数里的
	// persistMachineIDIfGenerated。
	machineID, _ := EnsureKiroMachineID(account.Credentials)

	header := kiro.BuildHeaders(kiro.HeaderOptions{
		Endpoint:    ep,
		BearerToken: account.KiroBearerToken(),
		MachineID:   machineID,
		IsAPIKey:    account.IsKiroAPIKeyAccount(),
		Profile:     s.profile(),
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("kiro: build request: %w", err)
	}
	req.Header = header

	hc, err := s.httpClientFor(ctx, account)
	if err != nil {
		return nil, err
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro: call %s: %w", ep.Name, err)
	}
	return resp, nil
}

// httpClientFor 返回按账号代理配置构建的客户端。
func (s *KiroGatewayService) httpClientFor(ctx context.Context, account *Account) (*http.Client, error) {
	proxyURL := s.resolveProxyURL(ctx, account)
	hc, err := httpclient.GetClient(httpclient.Options{
		ProxyURL: proxyURL,
		Timeout:  kiroUpstreamTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("kiro: build http client: %w", err)
	}
	return hc, nil
}

// resolveProxyURL 返回账号的代理地址：优先用已预加载的 account.Proxy，
// 未预加载时按 ProxyID 现查仓储兜底——对齐 GrokQuotaService.resolveProxyURL
// 的既有模式（真实账号测试后走查代码发现 Kiro 这条路径之前没有兜底，账号
// 配了代理但调用路径没预加载 Proxy 时会悄悄走直连，不是假设性风险）。
func (s *KiroGatewayService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	switch {
	case account.Proxy != nil:
		return account.Proxy.URL()
	case s != nil && s.proxyRepo != nil:
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			account.Proxy = proxy
			return proxy.URL()
		}
	}
	return ""
}
