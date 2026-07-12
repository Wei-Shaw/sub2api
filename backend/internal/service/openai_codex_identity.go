package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// codexUpstreamMinVersion 上游 /backend-api/codex 接受的最低 version 头：
// 若请求携带 version 且低于该值，上游直接 404（issue #3901，2026-07 实测）。
const codexUpstreamMinVersion = "0.144.0"

// enforceCodexIdentityHeaders 收口 OAuth（ChatGPT 内部接口）出站请求的客户端身份头。
// 上游要求 originator 与 User-Agent 首段配套且为官方客户端标识，version 头（若携带）
// 不低于 0.144.0，任一不满足即 404（issue #3901）。以最终 User-Agent 为准推导配套
// originator；推导不出官方身份（第三方 UA / UA 缺失）时整体回退为默认 Codex CLI 身份。
//
// 仅对携带 originator 的请求生效——compat messages bridge 故意不带 originator，保持原样。
// 必须在所有 User-Agent 改写（自定义 UA / ForceCodexCLI / 浏览器 UA 兜底）之后调用。
func enforceCodexIdentityHeaders(h http.Header) {
	if h == nil || h.Get("originator") == "" {
		return
	}
	originator, pairedUA, ok := openai.PairCodexClientIdentity(h.Get("user-agent"))
	if !ok {
		originator, pairedUA = "codex_cli_rs", codexCLIUserAgent
	}
	h.Set("user-agent", pairedUA)
	h.Set("originator", originator)
	if v := strings.TrimSpace(h.Get("version")); v != "" && CompareVersions(v, codexUpstreamMinVersion) < 0 {
		h.Set("version", codexCLIVersion)
	}
}
