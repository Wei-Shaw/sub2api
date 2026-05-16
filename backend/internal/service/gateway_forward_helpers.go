package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// ─────────────────────────────────────────────────────────────────────────────
// Forward 函数的子方法：将请求转发流程按阶段组织
// ─────────────────────────────────────────────────────────────────────────────

// forwardContext 封装 Forward 流程中各阶段共享的中间状态。
type forwardContext struct {
	body            []byte                  // 转换后的请求体
	originalModel   string                  // 原始模型名（用于计费和日志）
	mappedModel     string                  // 映射后的模型名（发送到上游）
	reqModel        string                  // 当前使用的模型名
	reqStream       bool                    // 是否流式请求
	mimicClaudeCode bool                    // 是否启用 Claude Code 伪装
	token           string                  // 认证凭证
	tokenType       string                  // 凭证类型 (oauth/apikey/setup-token)
	proxyURL        string                  // 代理 URL
	tlsProfile      *tlsfingerprint.Profile // TLS 指纹配置
	startTime       time.Time
}

// checkForwardEarlyRoutes 检查是否命中快速退出路径（web search、passthrough、bedrock）。
// 返回非nil结果表示已完成转发，主函数应直接返回。
func (s *GatewayService) checkForwardEarlyRoutes(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest, startTime time.Time) (*ForwardResult, error) {
	if account != nil && s.shouldEmulateWebSearch(ctx, account, parsed.GroupID, parsed.Body) {
		return s.handleWebSearchEmulation(ctx, c, account, parsed)
	}

	if account != nil && account.IsAnthropicAPIKeyPassthroughEnabled() {
		passthroughBody := parsed.Body
		passthroughModel := parsed.Model
		if passthroughModel != "" {
			if mappedModel := account.GetMappedModel(passthroughModel); mappedModel != passthroughModel {
				passthroughBody = s.replaceModelInBody(passthroughBody, mappedModel)
				logger.LegacyPrintf("service.gateway", "Passthrough model mapping: %s -> %s (account: %s)", parsed.Model, mappedModel, account.Name)
				passthroughModel = mappedModel
			}
		}
		return s.forwardAnthropicAPIKeyPassthroughWithInput(ctx, c, account, anthropicPassthroughForwardInput{
			Body:          passthroughBody,
			RequestModel:  passthroughModel,
			OriginalModel: parsed.Model,
			RequestStream: parsed.Stream,
			StartTime:     startTime,
		})
	}

	if account != nil && account.IsBedrock() {
		return s.forwardBedrock(ctx, c, account, parsed, startTime)
	}

	return nil, nil
}

// prepareForwardBody 执行请求体转换并获取凭证。
// 包括：Beta 策略评估、Claude Code 伪装、模型映射、cache 控制、凭证获取。
func (s *GatewayService) prepareForwardBody(ctx context.Context, c *gin.Context, account *Account, parsed *ParsedRequest, startTime time.Time) (*forwardContext, error) {
	// Beta 策略评估
	if account.Platform == PlatformAnthropic && c != nil {
		policy := s.evaluateBetaPolicy(ctx, c.GetHeader("anthropic-beta"), account, parsed.Model)
		if policy.blockErr != nil {
			return nil, policy.blockErr
		}
		filterSet := policy.filterSet
		if filterSet == nil {
			filterSet = map[string]struct{}{}
		}
		c.Set(betaPolicyFilterSetKey, filterSet)
	}

	body := parsed.Body
	reqModel := parsed.Model
	reqStream := parsed.Stream
	originalModel := reqModel

	if c != nil {
		s.debugLogGatewaySnapshot("CLIENT_ORIGINAL", c.Request.Header, body, map[string]string{
			"account":      fmt.Sprintf("%d(%s)", account.ID, account.Name),
			"account_type": string(account.Type),
			"model":        reqModel,
			"stream":       strconv.FormatBool(reqStream),
		})
	}

	// Claude Code 伪装判定
	isClaudeCode := IsClaudeCodeClient(ctx) || isClaudeCodeClient(c.GetHeader("User-Agent"), parsed.MetadataUserID)
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCode

	if shouldMimicClaudeCode {
		systemRewritten := false
		if !strings.Contains(strings.ToLower(reqModel), "haiku") {
			body = rewriteSystemForNonClaudeCode(body, parsed.System)
			systemRewritten = true
		}

		normalizeOpts := claudeOAuthNormalizeOptions{stripSystemCacheControl: !systemRewritten}
		if s.identityService != nil {
			fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, c.Request.Header)
			if err == nil && fp != nil {
				_, mimicMPT, _ := s.settingService.GetGatewayForwardingSettings(ctx)
				if !mimicMPT {
					if metadataUserID := s.buildOAuthMetadataUserID(parsed, account, fp); metadataUserID != "" {
						normalizeOpts.injectMetadata = true
						normalizeOpts.metadataUserID = metadataUserID
					}
				}
			}
		}

		body, reqModel = normalizeClaudeOAuthRequestBody(body, reqModel, normalizeOpts)

		body = s.rewriteMessageCacheControlIfEnabled(ctx, body)
		if rw := buildToolNameRewriteFromBody(body); rw != nil {
			body = applyToolNameRewriteToBody(body, rw)
			c.Set(toolNameRewriteKey, rw)
		} else {
			body = applyToolsLastCacheBreakpoint(body)
		}
	}

	body = enforceCacheControlLimit(body)

	// 模型映射
	mappedModel := reqModel
	mappingSource := ""
	if account.Type == AccountTypeAPIKey {
		mappedModel = account.GetMappedModel(reqModel)
		if mappedModel != reqModel {
			mappingSource = "account"
		}
	}
	if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		if candidate, matched := account.ResolveMappedModel(reqModel); matched {
			mappedModel = candidate
			mappingSource = "account"
		} else {
			normalized := normalizeVertexAnthropicModelID(claude.NormalizeModelID(reqModel))
			if normalized != reqModel {
				mappedModel = normalized
				mappingSource = "vertex"
			}
		}
	}
	if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(reqModel)
		if normalized != reqModel {
			mappedModel = normalized
			mappingSource = "prefix"
		}
	}
	if mappedModel != reqModel {
		body = s.replaceModelInBody(body, mappedModel)
		reqModel = mappedModel
		logger.LegacyPrintf("service.gateway", "Model mapping applied: %s -> %s (account: %s, source=%s)", originalModel, mappedModel, account.Name, mappingSource)
	}

	if s.shouldInjectAnthropicCacheTTL1h(ctx, account) {
		body = injectAnthropicCacheControlTTL1h(body)
	}

	// 获取凭证
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	// 解析代理
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			proxyURL = account.Proxy.URL()
		}
	}

	tlsProfile := s.tlsFPProfileService.ResolveTLSProfile(account)

	logger.LegacyPrintf("service.gateway", "[Forward] Using account: ID=%d Name=%s Platform=%s Type=%s TLSFingerprint=%v Proxy=%s",
		account.ID, account.Name, account.Platform, account.Type, tlsProfile, proxyURL)

	body = StripEmptyTextBlocks(body)
	setOpsUpstreamRequestBody(c, body)

	return &forwardContext{
		body:            body,
		originalModel:   originalModel,
		mappedModel:     mappedModel,
		reqModel:        reqModel,
		reqStream:       reqStream,
		mimicClaudeCode: shouldMimicClaudeCode,
		token:           token,
		tokenType:       tokenType,
		proxyURL:        proxyURL,
		tlsProfile:      tlsProfile,
		startTime:       startTime,
	}, nil
}

// processForwardResponse 处理上游响应并构建最终结果。
// 处理流式/非流式响应，构建 ForwardResult。
func (s *GatewayService) processForwardResponse(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, fc *forwardContext, parsed *ParsedRequest) (*ForwardResult, error) {
	// 触发上游接受回调
	if parsed.OnUpstreamAccepted != nil {
		parsed.OnUpstreamAccepted()
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	var err error

	if fc.reqStream {
		streamResult, streamErr := s.handleStreamingResponse(ctx, resp, c, account, fc.startTime, fc.originalModel, fc.reqModel, fc.mimicClaudeCode)
		if streamErr != nil {
			if streamErr.Error() == "have error in stream" {
				return nil, &UpstreamFailoverError{StatusCode: 403}
			}
			return nil, streamErr
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		usage, err = s.handleNonStreamingResponse(ctx, resp, c, account, fc.originalModel, fc.reqModel)
		if err != nil {
			return nil, err
		}
	}

	return &ForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            *usage,
		Model:            fc.originalModel,
		UpstreamModel:    fc.mappedModel,
		Stream:           fc.reqStream,
		Duration:         time.Since(fc.startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}
