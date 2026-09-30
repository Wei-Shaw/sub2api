//go:build live

package service

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestLiveOpenCodeFreeTierGate 是对真实 OpenCode Zen 上游的端到端验收测试。
//
// 它不模拟上游：用实现本身产出的门禁头与 body 直接打 https://opencode.ai，
// 并同时验证「无门禁必 403」与「有门禁必 200」，从而证明伪装真的生效，而不是
// 只在单元测试里自洽。
//
// 仅在显式指定构建标签时运行（不进入常规 CI）：
//
//	go test -tags=live ./internal/service/ -run TestLiveOpenCodeFreeTierGate -v -count=1
//
// 密钥默认取官方免费层公开值 public，可用 OPENCODE_ZEN_API_KEY 覆盖。
func TestLiveOpenCodeFreeTierGate(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENCODE_ZEN_API_KEY"))
	if apiKey == "" {
		apiKey = "public"
	}
	const endpoint = "https://opencode.ai/zen/v1/chat/completions"
	const model = "mimo-v2.6-flash-free"

	client := &http.Client{Timeout: 60 * time.Second}

	do := func(t *testing.T, headers http.Header, body []byte) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", openCodeUpstreamUserAgent)
		for key, values := range headers {
			for _, v := range values {
				req.Header.Set(key, v)
			}
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(raw)
	}

	account := openCodeGateTestAccount(nil)
	plain := []byte(`{"model":"` + model + `","stream":true,"messages":[{"role":"user","content":"say ok"}]}`)

	t.Run("对照组：无门禁必然被拒", func(t *testing.T) {
		status, body := do(t, http.Header{}, plain)
		require.Equal(t, http.StatusForbidden, status,
			"预期上游拒绝未伪装请求；若此处通过，说明上游门禁已放宽，本特性需要重新评估。响应: %s", body)
		require.Contains(t, body, "FreeTierError")
	})

	t.Run("实验组：门禁伪装必然放行", func(t *testing.T) {
		// 用真实实现产出 body 与头，而不是手写常量。
		gated, err := applyOpenCodeFreeTierGateBody(account, plain, openCodeGateDialectChat)
		require.NoError(t, err)
		require.True(t, gjson.GetBytes(gated, "stream").Bool(), "实现必须强制 stream:true")

		headers := http.Header{}
		// 模拟真实链路：applyOpenCodeSessionHeader 先落一个 UUID 值，
		// 再由门禁改写为规范形状。
		headers.Set(openCodeSessionHeader, "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
		applyOpenCodeFreeTierGateSession(account, headers, gated)
		require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, headers.Get(openCodeSessionHeader))

		status, body := do(t, headers, gated)
		require.Equal(t, http.StatusOK, status, "门禁伪装未通过上游校验，响应: %s", body)
		require.Contains(t, body, "data:", "预期收到 SSE 帧")
		require.NotContains(t, body, "FreeTierError")
	})

	t.Run("边界组：客户端不提供任何会话标识时也必须放行", func(t *testing.T) {
		// 这是实现过程中发现并修复的真实缺口：Zen 账号的
		// shouldGenerateOpenCodeSession 为 false（它只认 /zen/go 路径），
		// 因此 applyOpenCodeSessionHeader 不会补会话头，门禁必须自行派生。
		gated, err := applyOpenCodeFreeTierGateBody(account, plain, openCodeGateDialectChat)
		require.NoError(t, err)

		headers := http.Header{} // 刻意留空：模拟裸 curl / requests 客户端
		applyOpenCodeFreeTierGateSession(account, headers, gated)

		session := headers.Get(openCodeSessionHeader)
		require.Regexp(t, `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`, session,
			"门禁必须自行派生规范会话值")

		status, body := do(t, headers, gated)
		require.Equal(t, http.StatusOK, status,
			"无客户端会话标识时门禁仍未通过上游校验，响应: %s", body)
		require.NotContains(t, body, "FreeTierError")
	})
}
