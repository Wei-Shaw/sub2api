package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// EstimateKiroCountTokens 在本地估算一次 Anthropic 兼容 count_tokens 请求的
// input token，不发任何上游请求——Kiro 没有真实的 count_tokens 端点，
// gateway.go 的 countTokensHandler 之前对 kiro 分组落进
// GatewayHandler.CountTokens（Anthropic 真实上游转发），kiro 账号没有
// Anthropic 形态的凭证/endpoint，这条路径实际上打不通（真实账号复查发现，
// 不是假设性缺口）。
//
// 复用 kiro.EstimateRequestInput——计费路径（finishWithAction 之前的
// translator.SetInputTokens）已经在用同一套近似，不是新造一套算法。
// 对齐 Grok 的 EstimateGrokCountTokens（openai_gateway_count_tokens.go）
// 同类先例：本地估算、不选账号、不查计费资格。
func EstimateKiroCountTokens(body []byte) (int, error) {
	var req apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return 0, fmt.Errorf("kiro: parse count_tokens request: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return 0, fmt.Errorf("kiro: parse count_tokens request: model is required")
	}
	return kiro.EstimateRequestInput(&req), nil
}
