package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
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
func (s *AccountTestService) ProbeDeepseekToolsSupport(ctx context.Context, accountID int64) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_load_account_failed: account_id=%d err=%v", accountID, err)
		return
	}
	if account.Platform != PlatformDeepseek || account.Type != AccountTypeAPIKey {
		return
	}
	baseURL, _ := deepseekToolsBase(account, false)
	var supported *bool
	mode := strings.TrimSpace(account.GetExtraString(deepseekToolsModeExtraKey))
	if isDeepseekOfficialHost(baseURL) || mode == deepseekToolsModeForceSupported || mode == deepseekToolsModeForceUnsupported {
		if _, legacy := account.Extra[deepseekToolsSupportedExtraKey]; !legacy {
			return
		}
	} else {
		supported = s.probeDeepseekSearch(ctx, account)
	}
	store, ok := s.accountRepo.(interface {
		UpdateDeepseekToolsSupport(context.Context, *Account, *bool) error
	})
	if !ok {
		slog.Warn("deepseek_tools_probe_store_unavailable", "account_id", account.ID)
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := store.UpdateDeepseekToolsSupport(stateCtx, account, supported); err != nil {
		slog.Warn("deepseek_tools_probe_marker_persist_failed", "account_id", account.ID, "err", err)
	}
}

// nil means inconclusive and removes any previous observation.
func (s *AccountTestService) probeDeepseekSearch(ctx context.Context, account *Account) *bool {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		logger.LegacyPrintf("service.deepseek_probe", "probe_skip_no_apikey: account_id=%d", account.ID)
		return nil
	}
	probeURL, err := buildDeepseekToolsURL(account, "search", "", false, s.validateUpstreamBaseURL)
	if err != nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_build_url_failed: account_id=%d err=%v", account.ID, err)
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, deepseekToolsProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(probeCtx), http.MethodPost, probeURL.String(), strings.NewReader(deepseekToolsProbePayload))
	if err != nil {
		logger.LegacyPrintf("service.deepseek_probe", "probe_build_request_failed: account_id=%d err=%v", account.ID, err)
		return nil
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
		logger.LegacyPrintf("service.deepseek_probe", "probe_request_failed: account_id=%d err=%v", account.ID, err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		body, err := readUpstreamResponseBodyLimited(resp.Body, 1<<20)
		if err == nil && gjson.ValidBytes(body) && gjson.GetBytes(body, "results").IsArray() && gjson.GetBytes(body, "error").Type == gjson.Null {
			return new(true)
		}
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return new(false)
	}
	return nil
}
