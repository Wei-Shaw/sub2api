//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// 本文件覆盖 logPathMatchTrace（service_quota_precheck.go）输出。
// 用 slog.JSONHandler 写到 bytes.Buffer，然后逐行 json 解析校验字段——
// 比正则更稳，也避免对 slog text 默认格式的脆弱依赖。

// captureSlog 把全局 slog 默认 logger 临时切到 buf 上，返回还原函数。
// 调用方应 defer restore() 还原原 logger。
func captureSlog(t *testing.T, buf *bytes.Buffer) func() {
	t.Helper()
	prev := slog.Default()
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
	return func() { slog.SetDefault(prev) }
}

// ptrTo 把字面量值转成 *T，避开为每个测试声明一个临时变量。
func ptrTo[T any](v T) *T { return &v }

// captureMu 串行化 slog default logger 的替换 + 还原，避免 t.Parallel 下
// 多个测试同时切换全局 logger 互相覆盖。
var captureMu sync.Mutex

// readJSONLines 把 buf 中的每一行 JSON 解析回 map，方便按字段断言。
// 空行会被跳过。
func readJSONLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "解析 slog JSON: %s", line)
		out = append(out, m)
	}
	return out
}

// findFirstByMsg 返回 lines 中第一个 msg 等于 want 的 entry。
// 测试期间 PreCheckAcquire 也会触发 slog 输出 → 多条混合，按 msg 过滤更安全。
func findFirstByMsg(t *testing.T, lines []map[string]any, want string) map[string]any {
	t.Helper()
	for _, m := range lines {
		if msg, _ := m["msg"].(string); msg == want {
			return m
		}
	}
	t.Fatalf("没有找到 msg=%q 的日志，实际：%v", want, lines)
	return nil
}

// TestLogPathMatchTrace_Match：matched_paths>0 时输出 op+'.match' 与
// req.Platform/Channel/Account/Model 以及 matched_paths 字段。
func TestLogPathMatchTrace_Match(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	var buf bytes.Buffer
	defer captureSlog(t, &buf)()

	rule := &ServiceQuotaRule{
		ID:    99,
		Name:  ptrTo("rule-99"),
		Paths: []ServiceQuotaPathDef{{ID: 1}, {ID: 2}},
	}
	req := ServiceQuotaCheckRequest{
		Platform:  "anthropic",
		ChannelID: 11,
		AccountID: 22,
		Model:     "claude-opus",
	}
	logPathMatchTrace("svcquota.acquire", rule, []ServiceQuotaPathDef{{ID: 1}}, req)

	lines := readJSONLines(t, &buf)
	entry := findFirstByMsg(t, lines, "svcquota.acquire.match")
	require.EqualValues(t, 99, entry["rule_id"])
		require.EqualValues(t, 1, entry["matched_paths"])
	require.Equal(t, "anthropic", entry["req_platform"])
	require.EqualValues(t, 11, entry["req_channel"])
	require.EqualValues(t, 22, entry["req_account"])
	require.Equal(t, "claude-opus", entry["req_model"])
}

// TestLogPathMatchTrace_Miss：matched_paths==0 时输出 op+'.miss' 与
// req.* 以及 path_count（rule 配置的总 path 数）。
func TestLogPathMatchTrace_Miss(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	var buf bytes.Buffer
	defer captureSlog(t, &buf)()

	rule := &ServiceQuotaRule{
		ID:    100,
		Name:  ptrTo("rule-100"),
		Paths: []ServiceQuotaPathDef{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	req := ServiceQuotaCheckRequest{
		Platform:  "openai",
		ChannelID: 33,
		AccountID: 44,
		Model:     "gpt-5",
	}
	logPathMatchTrace("svcquota.record", rule, nil, req)

	lines := readJSONLines(t, &buf)
	entry := findFirstByMsg(t, lines, "svcquota.record.miss")
	require.EqualValues(t, 100, entry["rule_id"])
		require.EqualValues(t, 3, entry["path_count"], "miss 必须输出 rule 配置的 path 总数")
	require.Equal(t, "openai", entry["req_platform"])
	require.EqualValues(t, 33, entry["req_channel"])
	require.EqualValues(t, 44, entry["req_account"])
	require.Equal(t, "gpt-5", entry["req_model"])

	// 双向断言：miss 不应混入 .match 日志，避免 helper 同时打两条。
	for _, m := range lines {
		msg, _ := m["msg"].(string)
		require.NotEqual(t, "svcquota.record.match", msg, "miss 路径不应输出 match 日志")
	}
}

// TestLogPathMatchTrace_NilRule：rule==nil 时直接 return，不应 panic 也不写日志。
// 防御性测试：helper 当前已检查 rule==nil，但回归保护避免后续重构遗漏。
func TestLogPathMatchTrace_NilRule(t *testing.T) {
	captureMu.Lock()
	defer captureMu.Unlock()
	var buf bytes.Buffer
	defer captureSlog(t, &buf)()

	require.NotPanics(t, func() {
		logPathMatchTrace("svcquota.acquire", nil, nil, ServiceQuotaCheckRequest{})
	})
	require.Empty(t, strings.TrimSpace(buf.String()), "rule==nil 不应写日志")
	_ = context.Background() // 保留 context 包占位，让 helper 与生产签名贴近
}
