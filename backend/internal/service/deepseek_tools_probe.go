package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	// deepseekToolsProbeTimeout 是探测请求的超时时长。tools/search 是同步
	// 轻量端点，10 秒足够；超时则保持 unknown，由 host 兜底。
	deepseekToolsProbeTimeout = 10 * time.Second
	// deepseekToolsProbePayload 构造最小的探测请求体（按次计费约 0.005 元/次）。
	deepseekToolsProbePayload = `{"query":"probe","count":1}`
)

// ProbeDeepseekToolsSupport 探测 platform=deepseek 的 APIKey 账号上游是否支持
// DeepSeek 独立工具接口（/v1/tools/search 等），结果持久化到
// accounts.extra.deepseek_tools_supported。
//
// 调用时机：账号创建/更新后异步执行（见 admin.账户 handler 的
// scheduleOpenAIResponsesProbe）。幂等：重复调用以最新结果覆盖。
//
// 探测策略：
//   - 官方 api.deepseek.com（含未配置 base_url 的默认值）→ 直接落标 true，
//     不发网络请求（官方 API 已知支持，避免每次创建账号白花一次计费调用）。
//   - 已配置 deepseek_tools_mode 强制覆盖 → 跳过探测（覆盖优先，探测结果不可达）。
//   - 其他 base_url（ollama / crazyhub 等第三方中转）→ POST
//     {base}/v1/tools/search 最小请求：2xx → true；404/405 → false（端点不存在）；
//     其他非 2xx（401/403/429/5xx）与网络错误 → 不落标，保持 unknown，由
//     base_url host 兜底判定，避免把鉴权/限流问题误判为能力缺失。
//
// 失败处理：探测失败不阻塞账号创建/更新，仅记录日志。
func (s *AccountTestService) ProbeDeepseekToolsSupport(ctx context.Context, accountID int64) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_load_account_failed: account_id=%d err=%v", accountID, err)
		return
	}
	if account.Platform != PlatformDeepseek || account.Type != AccountTypeAPIKey {
		return
	}
	if _, ok := account.Extra[deepseekToolsModeExtraKey].(string); ok {
		// 强制覆盖优先，探测结果不可达，跳过（避免白跑一次计费调用）。
		return
	}
	baseURL, _ := deepseekToolsBase(account, false)
	if baseURL == "" || isDeepseekOfficialHost(baseURL) {
		s.persistDeepseekToolsSupport(ctx, account, true)
		return
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		logger.LegacyPrintf("service.deepseek_probe", "probe_skip_no_apikey: account_id=%d", accountID)
		return
	}
	probeURL, err := buildDeepseekToolsURL(account, "search", "", false, s.validateUpstreamBaseURL)
	if err != nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_build_url_failed: account_id=%d err=%v", accountID, err)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, deepseekToolsProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(probeCtx), http.MethodPost, probeURL.String(), bytes.NewReader([]byte(deepseekToolsProbePayload)))
	if err != nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_build_request_failed: account_id=%d err=%v", accountID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	// 账号级请求头覆写：探测与真实转发保持一致的最终头。
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		// 网络层失败：不写标记，保持 unknown，由 host 兜底。
		logger.LegacyPrintf("service.deepseek_probe", "probe_request_failed: account_id=%d url=%s err=%v", accountID, probeURL, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		s.persistDeepseekToolsSupport(ctx, account, true)
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		s.persistDeepseekToolsSupport(ctx, account, false)
	default:
		// 鉴权/限流/服务端错误不能证明能力缺失，保持 unknown。
		logger.LegacyPrintf("service.deepseek_probe", "probe_inconclusive: account_id=%d status=%d", accountID, resp.StatusCode)
	}
}

// persistDeepseekToolsSupport 把探测结论写入 accounts.extra，失败仅告警。
func (s *AccountTestService) persistDeepseekToolsSupport(ctx context.Context, account *Account, supported bool) {
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		deepseekToolsSupportedExtraKey: supported,
	}); err != nil {
		slog.Warn("deepseek_tools_probe_marker_persist_failed",
			"account_id", account.ID,
			"account_name", account.Name,
			"supported", supported,
			"err", err,
		)
	}
}
