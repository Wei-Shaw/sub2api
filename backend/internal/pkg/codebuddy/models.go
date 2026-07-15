package codebuddy

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// NonModelEntries 是 /v3/config 的 models / agents[].models 列表中不属于具体模型名的
// 占位项，不应作为可路由的模型暴露给 OpenAI 兼容网关。
var NonModelEntries = map[string]bool{
	"auto": true,
}

// DefaultModels 是当 /v3/config 拉取失败时的兜底模型列表（保守的常用模型）。
// 正常接入应通过 OAuth 登录后动态从 /v3/config 同步。
func DefaultModels() []string {
	return []string{
		"hy3",
		"glm-5.2",
		"glm-5.1",
		"deepseek-v4-flash",
		"deepseek-v4-pro",
		"kimi-k2.7",
		"minimax-m3",
	}
}

// ModelInfo 是解析后的单个 CodeBuddy 模型（含计费与上下文配置），
// 用于模型列表展示与网关 token 上限限制。
type ModelInfo struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	DisplayName       string  `json:"display_name"`
	Credits           string  `json:"credits"`
	CreditMultiplier  float64 `json:"credit_multiplier"`
	MaxInputTokens    int     `json:"max_input_tokens"`
	MaxOutputTokens   int     `json:"max_output_tokens"`
	SupportsImages    bool    `json:"supports_images"`
	SupportsReasoning bool    `json:"supports_reasoning"`
	SupportsToolCall  bool    `json:"supports_tool_call"`
}

// parseCreditMultiplier 将 "x2.20 credits" 形式的倍率字符串解析为 float。
// 空串、纯 "x0.00 credits" 或非数字返回 0。
func parseCreditMultiplier(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "x")
	s = strings.TrimSuffix(s, "credits")
	s = strings.TrimSuffix(s, "credit")
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseModels 从 /v3/config 解析完整模型列表（含计费与上下文配置）。
// 返回 data.models 全量，不过滤 auto 或可用性——用户要求展示后端返回的全部模型。
// 兼容顶层 {"models":[...]} 或 {"data":{"models":[...]}} 包装结构。
func ParseModels(configBody []byte) ([]ModelInfo, error) {
	if len(configBody) == 0 {
		return nil, nil
	}
	var cfg ConfigResponse
	if err := json.Unmarshal(configBody, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Agents) == 0 && len(cfg.Models) == 0 {
		var wrapped struct {
			Data ConfigResponse `json:"data"`
		}
		if err := json.Unmarshal(configBody, &wrapped); err == nil {
			cfg = wrapped.Data
		}
	}
	out := make([]ModelInfo, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		out = append(out, ModelInfo{
			ID:                m.ID,
			Name:              m.Name,
			DisplayName:       m.Name,
			Credits:           m.Credits,
			CreditMultiplier:  parseCreditMultiplier(m.Credits),
			MaxInputTokens:    m.MaxInputTokens,
			MaxOutputTokens:   m.MaxOutputTokens,
			SupportsImages:    m.SupportsImages,
			SupportsReasoning: m.SupportsReasoning,
			SupportsToolCall:  m.SupportsToolCall,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ParseEnabledModels 从 /v3/config 解析当前账号可调用的模型名列表。
// 同时聚合顶层 data.models 与 agents[].models（CodeBuddy 真实可用模型分散在两者），
// 过滤掉 NonModelEntries 中的占位项（如 auto），去重并按字典序排序。
// 兼容顶层 {"models":[...]} 或 {"data":{"models":[...]}} 包装结构。
func ParseEnabledModels(configBody []byte) ([]string, error) {
	if len(configBody) == 0 {
		return nil, nil
	}
	var cfg ConfigResponse
	if err := json.Unmarshal(configBody, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Agents) == 0 && len(cfg.Models) == 0 {
		var wrapped struct {
			Data ConfigResponse `json:"data"`
		}
		if err := json.Unmarshal(configBody, &wrapped); err == nil {
			cfg = wrapped.Data
		}
	}

	seen := make(map[string]struct{})
	models := make([]string, 0)
	addModel := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || NonModelEntries[name] {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		models = append(models, name)
	}
	for _, m := range cfg.Models {
		addModel(m.ID)
	}
	for _, agent := range cfg.Agents {
		for _, m := range agent.Models {
			addModel(m)
		}
	}
	sort.Strings(models)
	return models, nil
}
