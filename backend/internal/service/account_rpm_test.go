package service

import (
	"encoding/json"
	"testing"
)

func TestGetBaseRPM(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected int
	}{
		{"nil extra", nil, 0},
		{"no key", map[string]any{}, 0},
		{"zero", map[string]any{"base_rpm": 0}, 0},
		{"int value", map[string]any{"base_rpm": 15}, 15},
		{"float value", map[string]any{"base_rpm": 15.0}, 15},
		{"string value", map[string]any{"base_rpm": "15"}, 15},
		{"negative value", map[string]any{"base_rpm": -5}, 0},
		{"int64 value", map[string]any{"base_rpm": int64(20)}, 20},
		{"json.Number value", map[string]any{"base_rpm": json.Number("25")}, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetBaseRPM(); got != tt.expected {
				t.Errorf("GetBaseRPM() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetRPMStrategy(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected string
	}{
		{"nil extra", nil, "tiered"},
		{"no key", map[string]any{}, "tiered"},
		{"tiered", map[string]any{"rpm_strategy": "tiered"}, "tiered"},
		{"sticky_exempt", map[string]any{"rpm_strategy": "sticky_exempt"}, "sticky_exempt"},
		{"invalid", map[string]any{"rpm_strategy": "foobar"}, "tiered"},
		{"empty string fallback", map[string]any{"rpm_strategy": ""}, "tiered"},
		{"numeric value fallback", map[string]any{"rpm_strategy": 123}, "tiered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.GetRPMStrategy(); got != tt.expected {
				t.Errorf("GetRPMStrategy() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCheckRPMSchedulability(t *testing.T) {
	tests := []struct {
		name       string
		extra      map[string]any
		currentRPM int
		expected   WindowCostSchedulability
	}{
		{"disabled", map[string]any{}, 100, WindowCostSchedulable},
		{"green zone", map[string]any{"base_rpm": 15}, 10, WindowCostSchedulable},
		{"yellow zone tiered", map[string]any{"base_rpm": 15}, 15, WindowCostStickyOnly},
		{"red zone tiered", map[string]any{"base_rpm": 15}, 18, WindowCostNotSchedulable},
		{"sticky_exempt at limit", map[string]any{"base_rpm": 15, "rpm_strategy": "sticky_exempt"}, 15, WindowCostStickyOnly},
		{"sticky_exempt over limit", map[string]any{"base_rpm": 15, "rpm_strategy": "sticky_exempt"}, 100, WindowCostStickyOnly},
		{"custom buffer", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5}, 14, WindowCostStickyOnly},
		{"custom buffer red", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5}, 15, WindowCostNotSchedulable},
		{"base_rpm=1 green", map[string]any{"base_rpm": 1}, 0, WindowCostSchedulable},
		{"base_rpm=1 yellow (at limit)", map[string]any{"base_rpm": 1}, 1, WindowCostStickyOnly},
		{"base_rpm=1 red (at limit+buffer)", map[string]any{"base_rpm": 1}, 2, WindowCostNotSchedulable},
		{"negative currentRPM", map[string]any{"base_rpm": 15}, -1, WindowCostSchedulable},
		{"base_rpm negative disabled", map[string]any{"base_rpm": -5}, 10, WindowCostSchedulable},
		{"very high currentRPM", map[string]any{"base_rpm": 10}, 9999, WindowCostNotSchedulable},
		{"sticky_exempt very high currentRPM", map[string]any{"base_rpm": 10, "rpm_strategy": "sticky_exempt"}, 9999, WindowCostStickyOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extra}
			if got := a.CheckRPMSchedulability(tt.currentRPM); got != tt.expected {
				t.Errorf("CheckRPMSchedulability(%d) = %d, want %d", tt.currentRPM, got, tt.expected)
			}
		})
	}
}

func TestGetRPMStickyBuffer(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		extra       map[string]any
		expected    int
	}{
		// 基础退化
		{"nil extra", 0, nil, 0},
		{"no keys", 0, map[string]any{}, 0},
		{"base_rpm=0", 0, map[string]any{"base_rpm": 0}, 0},

		// 新公式: concurrency + maxSessions, floor = base/5
		{"conc=3 sess=10 → 13", 3, map[string]any{"base_rpm": 15, "max_sessions": 10}, 13},
		{"conc=2 sess=5 → 7", 2, map[string]any{"base_rpm": 10, "max_sessions": 5}, 7},
		{"conc=3 sess=15 → 18", 3, map[string]any{"base_rpm": 30, "max_sessions": 15}, 18},

		// floor 生效 (conc+sess < base/5)
		{"conc=0 sess=0 base=15 → floor 3", 0, map[string]any{"base_rpm": 15}, 3},
		{"conc=0 sess=0 base=10 → floor 2", 0, map[string]any{"base_rpm": 10}, 2},
		{"conc=0 sess=0 base=1 → floor 1", 0, map[string]any{"base_rpm": 1}, 1},
		{"conc=0 sess=0 base=4 → floor 1", 0, map[string]any{"base_rpm": 4}, 1},
		{"conc=1 sess=0 base=15 → floor 3", 1, map[string]any{"base_rpm": 15}, 3},

		// 手动 override
		{"custom buffer=5", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5, "max_sessions": 10}, 5},
		{"custom buffer=0 fallback", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 0, "max_sessions": 10}, 13},
		{"custom buffer negative fallback", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": -1, "max_sessions": 10}, 13},
		{"custom buffer with float", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": float64(7)}, 7},

		// 负值 clamp
		{"negative concurrency clamped", -5, map[string]any{"base_rpm": 15, "max_sessions": 10}, 10},
		{"negative maxSessions clamped", 3, map[string]any{"base_rpm": 15, "max_sessions": -5}, 3},

		// 高并发低会话
		{"conc=10 sess=5 → 15", 10, map[string]any{"base_rpm": 10, "max_sessions": 5}, 15},

		// json.Number
		{"json.Number base_rpm", 3, map[string]any{"base_rpm": json.Number("10"), "max_sessions": json.Number("5")}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Concurrency: tt.concurrency, Extra: tt.extra}
			if got := a.GetRPMStickyBuffer(); got != tt.expected {
				t.Errorf("GetRPMStickyBuffer() = %d, want %d", got, tt.expected)
			}
		})
	}
}
