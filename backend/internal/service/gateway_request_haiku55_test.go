//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// issue #7919：claude-haiku-5-5 请求校验（官方 migration guide 口径）。
func TestValidateClaude55Request_Haiku55(t *testing.T) {
	const model = "claude-haiku-5-5"
	const prefix = `{"model":"claude-haiku-5-5","max_tokens":16000,"messages":[{"role":"user","content":"hi"}]`
	cases := []struct {
		name string
		// extra 为追加进最小合法请求的 JSON 片段（含前导逗号）
		extra   string
		wantErr string // 为空表示期望通过
	}{
		{name: "adaptive thinking passes", extra: `,"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}`},
		{name: "omitted thinking passes", extra: ``},
		{name: "enabled thinking rejected", extra: `,"thinking":{"type":"enabled","budget_tokens":8000}`, wantErr: "requires adaptive thinking"},
		{name: "拒绝 between_tools 并返回 Haiku 错误说明", extra: `,"thinking":{"type":"between_tools"}`, wantErr: "claude-haiku-5-5 requires adaptive thinking; thinking.type=between_tools is unsupported"},
		{name: "disabled thinking allowed at medium effort", extra: `,"thinking":{"type":"disabled"},"output_config":{"effort":"medium"}`},
		{name: "disabled thinking allowed when effort omitted", extra: `,"thinking":{"type":"disabled"}`},
		{name: "disabled thinking rejected at xhigh", extra: `,"thinking":{"type":"disabled"},"output_config":{"effort":"xhigh"}`, wantErr: "supports only low, medium or high effort"},
		{name: "disabled thinking rejected at max", extra: `,"thinking":{"type":"disabled"},"output_config":{"effort":"max"}`, wantErr: "supports only low, medium or high effort"},
		{name: "temperature 1 passes", extra: `,"temperature":1`},
		{name: "non-default temperature rejected", extra: `,"temperature":0.5`, wantErr: "non-default temperature"},
		{name: "top_p 0.99 passes", extra: `,"top_p":0.99`},
		{name: "拒绝 top_p=0.995", extra: `,"top_p":0.995`, wantErr: "non-default top_p"},
		{name: "拒绝 top_p=1", extra: `,"top_p":1`, wantErr: "non-default top_p"},
		{name: "拒绝字符串 top_p", extra: `,"top_p":"0.99"`, wantErr: "non-default top_p"},
		{name: "top_p 0.5 rejected", extra: `,"top_p":0.5`, wantErr: "non-default top_p"},
		{name: "top_k rejected", extra: `,"top_k":5`, wantErr: "does not support top_k"},
		{name: "temperature and top_p together rejected", extra: `,"temperature":1,"top_p":0.99`, wantErr: "does not allow temperature and top_p together"},
		{name: "forced tool_choice any allowed", extra: `,"tool_choice":{"type":"any"}`},
		{name: "forced tool_choice required allowed", extra: `,"tool_choice":"required"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(prefix + tc.extra + "}")
			err := validateClaude55Request(body, model)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), tc.wantErr),
				"error %q should contain %q", err.Error(), tc.wantErr)
		})
	}
}
