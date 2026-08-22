package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	antigravityStickySessionTTL = time.Hour
	antigravityMaxRetries       = 3
	antigravityRetryBaseDelay   = 1 * time.Second
	antigravityRetryMaxDelay    = 16 * time.Second

	// 限流相关常量
	// antigravityRateLimitThreshold 限流等待/切换阈值
	// - 智能重试：retryDelay < 此阈值时等待后重试，>= 此阈值时直接限流模型
	// - 预检查：剩余限流时间 < 此阈值时等待，>= 此阈值时切换账号
	antigravityRateLimitThreshold       = 7 * time.Second
	antigravitySmartRetryMinWait        = 1 * time.Second  // 智能重试最小等待时间
	antigravitySmartRetryMaxAttempts    = 1                // 智能重试最大次数（仅重试 1 次，防止重复限流/长期等待）
	antigravityDefaultRateLimitDuration = 30 * time.Second // 默认限流时间（无 retryDelay 时使用）

	// MODEL_CAPACITY_EXHAUSTED 专用重试参数
	// 模型容量不足时，所有账号共享同一容量池，切换账号无意义
	// 使用固定 1s 间隔重试，最多重试 60 次
	antigravityModelCapacityRetryMaxAttempts = 60
	antigravityModelCapacityRetryWait        = 1 * time.Second

	// Google RPC 状态和类型常量
	googleRPCStatusResourceExhausted      = "RESOURCE_EXHAUSTED"
	googleRPCStatusUnavailable            = "UNAVAILABLE"
	googleRPCTypeRetryInfo                = "type.googleapis.com/google.rpc.RetryInfo"
	googleRPCTypeErrorInfo                = "type.googleapis.com/google.rpc.ErrorInfo"
	googleRPCReasonModelCapacityExhausted = "MODEL_CAPACITY_EXHAUSTED"
	googleRPCReasonRateLimitExceeded      = "RATE_LIMIT_EXCEEDED"

	// 单账号 503 退避重试：Service 层原地重试的最大次数
	// 在 handleSmartRetry 中，对于 shouldRateLimitModel（长延迟 ≥ 7s）的情况，
	// 多账号模式下会设限流+切换账号；但单账号模式下改为原地等待+重试。
	antigravitySingleAccountSmartRetryMaxAttempts = 3

	// 单账号 503 退避重试：原地重试时单次最大等待时间
	// 防止上游返回过长的 retryDelay 导致请求卡住太久
	antigravitySingleAccountSmartRetryMaxWait = 15 * time.Second

	// 单账号 503 退避重试：原地重试的总累计等待时间上限
	// 超过此上限将不再重试，直接返回 503
	antigravitySingleAccountSmartRetryTotalMaxWait = 30 * time.Second

	// MODEL_CAPACITY_EXHAUSTED 全局去重：重试全部失败后的 cooldown 时间
	antigravityModelCapacityCooldown = 10 * time.Second
)

// antigravityPassthroughErrorMessages 透传给客户端的错误消息白名单（小写）
// 匹配时使用 strings.Contains，无需完全匹配
var antigravityPassthroughErrorMessages = []string{
	"prompt is too long",
}

// MODEL_CAPACITY_EXHAUSTED 全局去重：避免多个并发请求同时对同一模型进行容量耗尽重试
var (
	modelCapacityExhaustedMu    sync.RWMutex
	modelCapacityExhaustedUntil = make(map[string]time.Time) // modelName -> cooldown until
)

const (
	antigravityForwardBaseURLEnv  = "GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL"
	antigravityFallbackSecondsEnv = "GATEWAY_ANTIGRAVITY_FALLBACK_COOLDOWN_SECONDS"
)

const antigravityProjectIDFallbackCredentialKey = "antigravity_project_id"

var errAntigravityProjectIDRequired = errors.New("该 standard-tier Antigravity 账号需配置 project_id")

// AntigravityAccountSwitchError 账号切换信号
// 当账号限流时间超过阈值时，通知上层切换账号
type AntigravityAccountSwitchError struct {
	OriginalAccountID int64
	RateLimitedModel  string
	IsStickySession   bool // 是否为粘性会话切换（决定是否缓存计费）
}

func (e *AntigravityAccountSwitchError) Error() string {
	return fmt.Sprintf("account %d model %s rate limited, need switch",
		e.OriginalAccountID, e.RateLimitedModel)
}

// IsAntigravityAccountSwitchError 检查错误是否为账号切换信号
func IsAntigravityAccountSwitchError(err error) (*AntigravityAccountSwitchError, bool) {
	var switchErr *AntigravityAccountSwitchError
	if errors.As(err, &switchErr) {
		return switchErr, true
	}
	return nil, false
}

// PromptTooLongError 表示上游明确返回 prompt too long
type PromptTooLongError struct {
	StatusCode int
	RequestID  string
	Body       []byte
}

func (e *PromptTooLongError) Error() string {
	return fmt.Sprintf("prompt too long: status=%d", e.StatusCode)
}

// AntigravityGatewayService 处理 Antigravity 平台的 API 转发
type AntigravityGatewayService struct {
	accountRepo       AccountRepository
	tokenProvider     *AntigravityTokenProvider
	rateLimitService  *RateLimitService
	httpUpstream      HTTPUpstream
	settingService    *SettingService
	cache             GatewayCache // 用于模型级限流时清除粘性会话绑定
	schedulerSnapshot *SchedulerSnapshotService
	internal500Cache  Internal500CounterCache // INTERNAL 500 渐进惩罚计数器
}

func (s *AntigravityGatewayService) upstreamErrorBodyReadLimit() int64 {
	limit := gatewayUpstreamErrorBodyReadLimit
	if s != nil && s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.LogUpstreamErrorBody && s.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes > int(limit) {
		limit = int64(s.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
	}
	return limit
}

func (s *AntigravityGatewayService) readUpstreamErrorBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, s.upstreamErrorBodyReadLimit()))
	return body
}

func NewAntigravityGatewayService(
	accountRepo AccountRepository,
	cache GatewayCache,
	schedulerSnapshot *SchedulerSnapshotService,
	tokenProvider *AntigravityTokenProvider,
	rateLimitService *RateLimitService,
	httpUpstream HTTPUpstream,
	settingService *SettingService,
	internal500Cache Internal500CounterCache,
) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		accountRepo:       accountRepo,
		tokenProvider:     tokenProvider,
		rateLimitService:  rateLimitService,
		httpUpstream:      httpUpstream,
		settingService:    settingService,
		cache:             cache,
		schedulerSnapshot: schedulerSnapshot,
		internal500Cache:  internal500Cache,
	}
}

// GetTokenProvider 返回 token provider
func (s *AntigravityGatewayService) GetTokenProvider() *AntigravityTokenProvider {
	return s.tokenProvider
}

// getLogConfig 获取上游错误日志配置
// 返回是否记录日志体和最大字节数
func (s *AntigravityGatewayService) getLogConfig() (logBody bool, maxBytes int) {
	maxBytes = 2048 // 默认值
	if s.settingService == nil || s.settingService.cfg == nil {
		return false, maxBytes
	}
	cfg := s.settingService.cfg.Gateway
	if cfg.LogUpstreamErrorBodyMaxBytes > 0 {
		maxBytes = cfg.LogUpstreamErrorBodyMaxBytes
	}
	return cfg.LogUpstreamErrorBody, maxBytes
}

// getUpstreamErrorDetail 获取上游错误详情（用于日志记录）
func (s *AntigravityGatewayService) getUpstreamErrorDetail(body []byte) string {
	logBody, maxBytes := s.getLogConfig()
	if !logBody {
		return ""
	}
	return truncateString(string(body), maxBytes)
}

// checkErrorPolicy nil 安全的包装
func (s *AntigravityGatewayService) checkErrorPolicy(ctx context.Context, account *Account, statusCode int, body []byte, requestedModel ...string) ErrorPolicyResult {
	if s.rateLimitService == nil {
		return ErrorPolicyNone
	}
	return s.rateLimitService.CheckErrorPolicy(ctx, account, statusCode, body, firstRequestedModel(requestedModel))
}

// applyErrorPolicy 应用错误策略结果，返回是否应终止当前循环及应返回的状态码。
// ErrorPolicySkipped 时 outStatus 为 500（前端约定：未命中的错误返回 500）。
func (s *AntigravityGatewayService) applyErrorPolicy(p antigravityRetryLoopParams, statusCode int, headers http.Header, respBody []byte) (handled bool, outStatus int, retErr error) {
	modelKey := resolveFinalAntigravityModelKey(p.ctx, p.account, p.requestedModel)
	switch s.checkErrorPolicy(p.ctx, p.account, statusCode, respBody, modelKey) {
	case ErrorPolicySkipped:
		if s.handleAntigravityModelRateLimitBeforePolicy(p, statusCode, headers, respBody) {
			return true, statusCode, nil
		}
		return true, http.StatusInternalServerError, nil
	case ErrorPolicyMatched:
		if s.handleAntigravityModelRateLimitBeforePolicy(p, statusCode, headers, respBody) {
			return true, statusCode, nil
		}
		_ = p.handleError(p.ctx, p.prefix, p.account, statusCode, headers, respBody,
			p.requestedModel, p.groupID, p.sessionHash, p.isStickySession)
		return true, statusCode, nil
	case ErrorPolicyTempUnscheduled:
		slog.Info("temp_unschedulable_matched",
			"prefix", p.prefix, "status_code", statusCode, "account_id", p.account.ID)
		return true, statusCode, &AntigravityAccountSwitchError{OriginalAccountID: p.account.ID, RateLimitedModel: p.requestedModel, IsStickySession: p.isStickySession}
	}
	return false, statusCode, nil
}

func (s *AntigravityGatewayService) handleAntigravityModelRateLimitBeforePolicy(p antigravityRetryLoopParams, statusCode int, headers http.Header, respBody []byte) bool {
	if statusCode != http.StatusTooManyRequests && statusCode != http.StatusServiceUnavailable {
		return false
	}
	if p.account == nil || p.account.Platform != PlatformAntigravity {
		return false
	}
	_, shouldRateLimitModel, waitDuration, modelName, isModelCapacityExhausted := shouldTriggerAntigravitySmartRetry(p.account, respBody)
	if isModelCapacityExhausted || !shouldRateLimitModel || strings.TrimSpace(modelName) == "" {
		return false
	}
	rateLimitDuration := waitDuration
	if rateLimitDuration <= 0 {
		rateLimitDuration = antigravityDefaultRateLimitDuration
	}
	resetAt := time.Now().Add(rateLimitDuration)
	if !s.setAntigravityModelRateLimits(p.ctx, p.accountRepo, p.account, modelName, p.prefix, statusCode, resetAt, false) {
		return false
	}
	s.clearStickySession(p.ctx, p.groupID, p.sessionHash)
	logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d model_rate_limited_before_error_policy model=%s account=%d reset_in=%v",
		p.prefix, statusCode, modelName, p.account.ID, rateLimitDuration)
	return true
}

// mapAntigravityModel 获取映射后的模型名
// 完全依赖映射配置：账户映射（通配符）→ 默认映射兜底（DefaultAntigravityModelMapping）
// 注意：返回空字符串表示模型不被支持，调度时会过滤掉该账号
func mapAntigravityModel(account *Account, requestedModel string) string {
	if account == nil {
		return ""
	}
	requestedModel = strings.TrimPrefix(requestedModel, "models/")

	// 获取映射表（未配置时自动使用 DefaultAntigravityModelMapping）
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return "" // 无映射配置（非 Antigravity 平台）
	}

	// 通过映射表查询（支持精确匹配 + 通配符）
	mapped := account.GetMappedModel(requestedModel)

	// 判断是否映射成功（mapped != requestedModel 说明找到了映射规则）
	if mapped != requestedModel {
		return mapped
	}

	// 如果 mapped == requestedModel，检查是否在映射表中配置（精确或通配符）
	// 这区分两种情况：
	// 1. 映射表中有 "model-a": "model-a"（显式透传）→ 返回 model-a
	// 2. 通配符匹配 "claude-*": "claude-sonnet-4-5" 恰好目标等于请求名 → 返回 model-a
	// 3. 映射表中没有 model-a 的配置 → 返回空（不支持）
	if account.IsModelSupported(requestedModel) {
		return requestedModel
	}

	// 未在映射表中配置的模型，返回空字符串（不支持）
	return ""
}

// getMappedModel 获取映射后的模型名
// 完全依赖映射配置：账户映射（通配符）→ 默认映射兜底
func (s *AntigravityGatewayService) getMappedModel(account *Account, requestedModel string) string {
	return mapAntigravityModel(account, requestedModel)
}

func resolveAntigravityProjectID(account *Account) (string, error) {
	if account == nil {
		return "", errAntigravityProjectIDRequired
	}
	if projectID := strings.TrimSpace(account.GetCredential("project_id")); projectID != "" {
		return projectID, nil
	}
	if projectID := strings.TrimSpace(account.GetCredential(antigravityProjectIDFallbackCredentialKey)); projectID != "" {
		return projectID, nil
	}
	if projectID := strings.TrimSpace(account.GetExtraString(antigravityProjectIDFallbackCredentialKey)); projectID != "" {
		return projectID, nil
	}
	return "", errAntigravityProjectIDRequired
}

// applyThinkingModelSuffix 根据 thinking 配置调整模型名
// 当映射结果是 claude-sonnet-4-5 且请求开启了 thinking 时，改为 claude-sonnet-4-5-thinking
func applyThinkingModelSuffix(mappedModel string, thinkingEnabled bool) string {
	if !thinkingEnabled {
		return mappedModel
	}
	if mappedModel == "claude-sonnet-4-5" {
		return "claude-sonnet-4-5-thinking"
	}
	return mappedModel
}

// IsModelSupported 检查模型是否被支持
// 所有 claude- 和 gemini- 前缀的模型都能通过映射或透传支持
func (s *AntigravityGatewayService) IsModelSupported(requestedModel string) bool {
	return strings.HasPrefix(requestedModel, "claude-") ||
		strings.HasPrefix(requestedModel, "gemini-")
}

// TestConnectionResult 测试连接结果
type TestConnectionResult struct {
	Text        string // 响应文本
	MappedModel string // 实际使用的模型
}

// TestConnection 测试 Antigravity 账号连接。
// 复用 antigravityRetryLoop 的完整重试 / credits overages / 智能重试逻辑，
// 与真实调度行为一致。差异：不做账号切换（测试指定账号）、不记录 ops 错误。
func (s *AntigravityGatewayService) TestConnection(ctx context.Context, account *Account, modelID string) (*TestConnectionResult, error) {

	// 获取 token
	if s.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
	}

	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		return nil, err
	}

	// 模型映射
	mappedModel := s.getMappedModel(account, modelID)
	if mappedModel == "" {
		return nil, fmt.Errorf("model %s not in whitelist", modelID)
	}

	// 构建请求体
	var requestBody []byte
	if strings.HasPrefix(modelID, "gemini-") {
		requestBody, err = s.buildGeminiTestRequest(projectID, mappedModel)
	} else {
		requestBody, err = s.buildClaudeTestRequest(projectID, mappedModel)
	}
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	// 代理 URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 复用 antigravityRetryLoop：完整的重试 / credits overages / 智能重试
	prefix := fmt.Sprintf("[antigravity-Test] account=%d(%s)", account.ID, account.Name)
	p := antigravityRetryLoopParams{
		ctx:            ctx,
		prefix:         prefix,
		account:        account,
		proxyURL:       proxyURL,
		accessToken:    accessToken,
		action:         "streamGenerateContent",
		body:           requestBody,
		c:              nil, // 无 gin.Context → 跳过 ops 追踪
		httpUpstream:   s.httpUpstream,
		settingService: s.settingService,
		accountRepo:    s.accountRepo,
		requestedModel: modelID,
		handleError:    testConnectionHandleError,
	}

	result, err := s.antigravityRetryLoop(p)
	if err != nil {
		// AccountSwitchError → 测试时不切换账号，返回友好提示
		var switchErr *AntigravityAccountSwitchError
		if errors.As(err, &switchErr) {
			return nil, fmt.Errorf("该账号模型 %s 当前限流中，请稍后重试", switchErr.RateLimitedModel)
		}
		return nil, err
	}

	if result == nil || result.resp == nil {
		return nil, errors.New("upstream returned empty response")
	}
	defer func() { _ = result.resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(result.resp.Body, s.upstreamErrorBodyReadLimit()))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if result.resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API 返回 %d: %s", result.resp.StatusCode, string(respBody))
	}

	text := extractTextFromSSEResponse(respBody)
	return &TestConnectionResult{Text: text, MappedModel: mappedModel}, nil
}

// testConnectionHandleError 是 TestConnection 使用的轻量 handleError 回调。
// 仅记录日志，不做 ops 错误追踪或粘性会话清除。
func testConnectionHandleError(
	_ context.Context, prefix string, account *Account,
	statusCode int, _ http.Header, body []byte,
	requestedModel string, _ int64, _ string, _ bool,
) *handleModelRateLimitResult {
	logger.LegacyPrintf("service.antigravity_gateway",
		"%s test_handle_error status=%d model=%s account=%d body=%s",
		prefix, statusCode, requestedModel, account.ID, truncateForLog(body, 200))
	return nil
}

// buildGeminiTestRequest 构建 Gemini 格式测试请求
// 使用最小 token 消耗：输入 "." + maxOutputTokens: 1
func (s *AntigravityGatewayService) buildGeminiTestRequest(projectID, model string) ([]byte, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "."},
				},
			},
		},
		// Antigravity 上游要求必须包含身份提示词
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": antigravity.GetDefaultIdentityPatch()},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1,
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	return s.wrapV1InternalRequest(projectID, model, payloadBytes)
}

// buildClaudeTestRequest 构建 Claude 格式测试请求并转换为 Gemini 格式
// 使用最小 token 消耗：输入 "." + MaxTokens: 1
func (s *AntigravityGatewayService) buildClaudeTestRequest(projectID, mappedModel string) ([]byte, error) {
	claudeReq := &antigravity.ClaudeRequest{
		Model: mappedModel,
		Messages: []antigravity.ClaudeMessage{
			{
				Role:    "user",
				Content: json.RawMessage(`"."`),
			},
		},
		MaxTokens: 1,
		Stream:    false,
	}
	return antigravity.TransformClaudeToGemini(claudeReq, projectID, mappedModel)
}

func (s *AntigravityGatewayService) getClaudeTransformOptions(ctx context.Context) antigravity.TransformOptions {
	opts := antigravity.DefaultTransformOptions()
	if s.settingService == nil {
		return opts
	}
	opts.EnableIdentityPatch = s.settingService.IsIdentityPatchEnabled(ctx)
	opts.IdentityPatch = s.settingService.GetIdentityPatchPrompt(ctx)
	return opts
}

// extractTextFromSSEResponse 从 SSE 流式响应中提取文本
func extractTextFromSSEResponse(respBody []byte) string {
	var texts []string
	lines := bytes.Split(respBody, []byte("\n"))

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// 跳过 SSE 前缀
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimPrefix(line, []byte("data:"))
			line = bytes.TrimSpace(line)
		}

		// 跳过非 JSON 行
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		// 解析 JSON
		var data map[string]any
		if err := json.Unmarshal(line, &data); err != nil {
			continue
		}

		// 尝试从 response.candidates[0].content.parts[].text 提取
		response, ok := data["response"].(map[string]any)
		if !ok {
			// 尝试直接从 candidates 提取（某些响应格式）
			response = data
		}

		candidates, ok := response["candidates"].([]any)
		if !ok || len(candidates) == 0 {
			continue
		}

		candidate, ok := candidates[0].(map[string]any)
		if !ok {
			continue
		}

		content, ok := candidate["content"].(map[string]any)
		if !ok {
			continue
		}

		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}

		for _, part := range parts {
			if partMap, ok := part.(map[string]any); ok {
				if text, ok := partMap["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			}
		}
	}

	return strings.Join(texts, "")
}

// injectIdentityPatchToGeminiRequest 为 Gemini 格式请求注入身份提示词
// 如果请求中已包含 "You are Antigravity" 则不重复注入
func injectIdentityPatchToGeminiRequest(body []byte) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("解析 Gemini 请求失败: %w", err)
	}

	// 检查现有 systemInstruction 是否已包含身份提示词
	if sysInst, ok := request["systemInstruction"].(map[string]any); ok {
		if parts, ok := sysInst["parts"].([]any); ok {
			for _, part := range parts {
				if partMap, ok := part.(map[string]any); ok {
					if text, ok := partMap["text"].(string); ok {
						if strings.Contains(text, "You are Antigravity") {
							// 已包含身份提示词，直接返回原始请求
							return body, nil
						}
					}
				}
			}
		}
	}

	// 获取默认身份提示词
	identityPatch := antigravity.GetDefaultIdentityPatch()

	// 构建新的 systemInstruction
	newPart := map[string]any{"text": identityPatch}

	if existing, ok := request["systemInstruction"].(map[string]any); ok {
		// 已有 systemInstruction，在开头插入身份提示词
		if parts, ok := existing["parts"].([]any); ok {
			existing["parts"] = append([]any{newPart}, parts...)
		} else {
			existing["parts"] = []any{newPart}
		}
	} else {
		// 没有 systemInstruction，创建新的
		request["systemInstruction"] = map[string]any{
			"parts": []any{newPart},
		}
	}

	return json.Marshal(request)
}

// wrapV1InternalRequest 包装请求为 v1internal 格式
func (s *AntigravityGatewayService) wrapV1InternalRequest(projectID, model string, originalBody []byte) ([]byte, error) {
	var request any
	if err := json.Unmarshal(originalBody, &request); err != nil {
		return nil, fmt.Errorf("解析请求体失败: %w", err)
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errAntigravityProjectIDRequired
	}

	wrapped := map[string]any{
		"project":     projectID,
		"requestId":   "agent-" + uuid.New().String(),
		"userAgent":   "antigravity", // 固定值，与官方客户端一致
		"requestType": "agent",
		"model":       model,
		"request":     request,
	}

	return json.Marshal(wrapped)
}

// unwrapV1InternalResponse 解包 v1internal 响应
// 使用 gjson 零拷贝提取 response 字段，避免 Unmarshal+Marshal 双重开销
func (s *AntigravityGatewayService) unwrapV1InternalResponse(body []byte) ([]byte, error) {
	result := gjson.GetBytes(body, "response")
	if result.Exists() {
		return []byte(result.Raw), nil
	}
	return body, nil
}
