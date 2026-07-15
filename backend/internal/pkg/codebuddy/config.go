package codebuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// configUserAgent 是 CodeBuddy 接口约定的 User-Agent（与 OAuth 客户端保持一致）。
const configUserAgent = "WorkBuddy/5.2.5 WorkBuddy/5.2.5 CLI/2.106.4"

// ConfigEnvelope 是 /v3/config 等接口的统一响应信封。
type ConfigEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// FetchEnabledModels 从 /v3/config 实时拉取当前账号可用的真实模型列表（含计费与上下文配置）。
// accessToken 为 CodeBuddy OAuth access_token（作为 Bearer 鉴权）；
// userID 对应账号 uid（X-User-Id 头，可选）；proxyURL 可选（直连或走代理）。
// 返回去重、排序后的 ModelInfo 切片；拉取失败时返回错误，由调用方决定回落策略。
func FetchEnabledModels(ctx context.Context, accessToken, userID, proxyURL string) ([]ModelInfo, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("codebuddy config: empty access token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BuildConfigURL(DefaultBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build config request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("x-domain", DefaultDomain)
	req.Header.Set("x-product", "SaaS")
	req.Header.Set("user-agent", configUserAgent)
	if strings.TrimSpace(userID) != "" {
		req.Header.Set("x-user-id", strings.TrimSpace(userID))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if proxyURL != "" {
		if transport, terr := newProxyTransport(proxyURL); terr == nil {
			client.Transport = transport
		} else {
			slog.Warn("codebuddy config: invalid proxy url, falling back to direct connection",
				"proxy_url", proxyURL, "error", terr)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codebuddy config request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read config response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codebuddy config returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var env ConfigEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse config envelope: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("codebuddy config error code=%d msg=%s", env.Code, env.Msg)
	}

	return ParseModels(env.Data)
}
