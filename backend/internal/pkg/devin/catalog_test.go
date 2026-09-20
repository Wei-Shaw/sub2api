// catalog_test.go 验证分组模型解析与 effort 归一化。
package devin

import "testing"

func groupedFixture() []GroupedModel {
	return []GroupedModel{
		{
			ID:   "swe-2",
			Name: "SWE-2",
			ThinkingLevelMap: map[ThinkingLevel]string{
				ThinkingOff:  "swe-2-none",
				ThinkingLow:  "swe-2-low",
				ThinkingHigh: "swe-2-high",
				ThinkingMax:  "swe-2-max",
			},
		},
	}
}

func TestNormalizeEffortParam(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"none":     "off",
		"NONE":     "off",
		"disabled": "off",
		"enabled":  "high",
		"on":       "high",
		"x-high":   "xhigh",
		"x_high":   "xhigh",
		"MAX":      "max",
		"garbage":  "garbage",
	}
	for in, want := range cases {
		if got := NormalizeEffortParam(in); got != want {
			t.Errorf("NormalizeEffortParam(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveCatalogModelUID(t *testing.T) {
	groups := groupedFixture()
	cases := []struct {
		model, effort, want string
	}{
		{"swe-2", "", "swe-2-high"}, // 默认 high
		{"swe-2", "max", "swe-2-max"},
		{"swe-2", "none", "swe-2-none"},   // OpenAI none → off
		{"swe-2:max", "", "swe-2-max"},    // 后缀语法
		{"swe-2:low", "max", "swe-2-max"}, // 显式 effort 优先于后缀
		{"swe-2", "x-high", "swe-2-max"},  // x-high→xhigh 无 uid，向上命中 max
		{"swe-2", "medium", "swe-2-high"}, // medium 无 uid：先向上命中 high
		{"raw-uid-9", "", "raw-uid-9"},    // 非分组 id 透传
	}
	for _, c := range cases {
		if got := ResolveCatalogModelUID(groups, c.model, c.effort); got != c.want {
			t.Errorf("ResolveCatalogModelUID(%q,%q) = %q, want %q", c.model, c.effort, got, c.want)
		}
	}
}
