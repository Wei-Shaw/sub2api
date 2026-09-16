package main

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestParsePreviewInputsIgnoresNullUserOverride(t *testing.T) {
	raw := []byte(`{
		"groups": [{
			"group": {
				"id": 50,
				"name": "preview",
				"platform": "openai",
				"rate_multiplier": 0.5,
				"subscription_type": "standard",
				"profit_control_enabled": false,
				"profit_min_margin": 0.1,
				"profit_safety_buffer": 0
			},
			"accounts": [{
				"id": 1,
				"name": "cheap",
				"platform": "openai",
				"type": "apikey",
				"rate_multiplier": 0.2
			}],
			"user_overrides": {"40": null, "41": 0.4},
			"models": ["gpt-test"]
		}]
	}`)

	inputs, err := parsePreviewInputs(raw, true)
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	require.Equal(t, map[int64]float64{41: 0.4}, inputs[0].UserOverrides)
	require.True(t, inputs[0].AssumeEnabled)

	report := service.PreviewProfitAdmission(inputs, time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC))[0]
	require.InDelta(t, 0.4, report.MinEffectiveD, 1e-12, "null 覆盖不能被解码成 0 倍率")
	require.InDelta(t, 0.36, report.ThresholdMinD, 1e-12)
}

func TestParsePreviewInputsRejectsEmptyGroups(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"groups":null}`),
		[]byte(`{"groups":[]}`),
	} {
		inputs, err := parsePreviewInputs(raw, false)
		require.ErrorContains(t, err, "input contains no groups")
		require.Nil(t, inputs)
	}
}

// TestModelsWithZeroRemainingWarnings 钉死两档 D 的归零告警分工：
// 默认 D 归零由 modelsWithZeroRemaining 报告；默认 D 仍有账号但最低有效 D
// （分组内存在更低的用户级倍率覆盖）归零的模型必须单独告警——那些用户的该
// 模型会全黑，只看默认 D 完全看不出来。两档都为 0 时不重复告警。
func TestModelsWithZeroRemainingWarnings(t *testing.T) {
	report := service.ProfitPreviewGroupReport{
		RemainingByModel: map[string]int{
			"both-zero":      0,
			"min-d-zero":     2,
			"healthy":        3,
			"min-d-zero-alt": 1,
		},
		RemainingByModelMinD: map[string]int{
			"both-zero":      0,
			"min-d-zero":     0,
			"healthy":        3,
			"min-d-zero-alt": 0,
		},
	}

	if got := modelsWithZeroRemaining(report); len(got) != 1 || got[0] != "both-zero" {
		t.Fatalf("默认D归零告警应只覆盖 both-zero，got %v", got)
	}

	got := modelsWithZeroRemainingUnderMinD(report)
	want := []string{"min-d-zero", "min-d-zero-alt"}
	if len(got) != len(want) {
		t.Fatalf("最低有效D归零告警不符: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("最低有效D归零告警不符（应按模型名排序）: got %v want %v", got, want)
		}
	}
}
