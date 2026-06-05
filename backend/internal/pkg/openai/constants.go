// Package openai provides helpers and types for OpenAI API integration.
package openai

import (
	_ "embed"
	"strings"
)

// Model represents an OpenAI model
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// DefaultModels OpenAI models list
var DefaultModels = []Model{
	{ID: "gpt-5.5", Object: "model", Created: 1776873600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.5"},
	{ID: "gpt-5.4", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4"},
	{ID: "gpt-5.4-mini", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.4 Mini"},
	{ID: "gpt-5.3-codex", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex"},
	{ID: "gpt-5.3-codex-spark", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.3 Codex Spark"},
	{ID: "gpt-5.2", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.2"},
	{ID: "gpt-image-1", Object: "model", Created: 1733875200, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 1"},
	{ID: "gpt-image-1.5", Object: "model", Created: 1735689600, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 1.5"},
	{ID: "gpt-image-2", Object: "model", Created: 1738368000, OwnedBy: "openai", Type: "model", DisplayName: "GPT Image 2"},
}

// DefaultModelIDs returns the default model ID list
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel default model for testing OpenAI accounts
const DefaultTestModel = "gpt-5.4"

// DefaultInstructions default instructions for non-Codex CLI requests.
// 内容为真实 Codex CLI 的 GPT-5-Codex base prompt（codex 系模型默认）。
//
//go:embed instructions.txt
var DefaultInstructions string

// instructionsGPT51 / instructionsGPT52 为 gpt-5.1 / gpt-5.2 非 codex 模型对应的
// 真实 Codex 编码 agent base prompt，用于模型感知的 instructions 选择。
//
//go:embed instructions_gpt5_1.txt
var instructionsGPT51 string

//go:embed instructions_gpt5_2.txt
var instructionsGPT52 string

// CodexBaseInstructionsForModel 按模型返回最匹配的真实 Codex base instructions：
//   - 含 "codex" 的模型（gpt-5-codex / gpt-5.x-codex / codex-max / spark 等）→ GPT-5-Codex prompt
//   - gpt-5.2 系非 codex 模型 → GPT-5.2 prompt
//   - gpt-5.1 / gpt-5 系非 codex 模型 → GPT-5.1 prompt
//   - 其它 → 回退到 GPT-5-Codex prompt
//
// 任一专用 prompt 意外为空时回退到 DefaultInstructions，保证返回非空。
func CodexBaseInstructionsForModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "codex"):
		return DefaultInstructions
	case strings.HasPrefix(m, "gpt-5.2"):
		if v := strings.TrimSpace(instructionsGPT52); v != "" {
			return instructionsGPT52
		}
	case strings.HasPrefix(m, "gpt-5.1"), strings.HasPrefix(m, "gpt-5"):
		if v := strings.TrimSpace(instructionsGPT51); v != "" {
			return instructionsGPT51
		}
	}
	return DefaultInstructions
}
