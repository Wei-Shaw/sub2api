package service

import "testing"

// TestNormalizeGroupPlatformDefaultsToAnthropic 钉死创建分组时省略 platform 的
// 归一化结果：handler 的利润控制预校验与 CreateGroup 落库共用这个函数，
// 两边一旦漂移，「省略 platform + profit_control_enabled=true」的创建请求
// 会被 handler 以「平台不支持」400 掉，而该分组本会建成受支持的 anthropic。
func TestNormalizeGroupPlatformDefaultsToAnthropic(t *testing.T) {
	if got := NormalizeGroupPlatform(""); got != PlatformAnthropic {
		t.Fatalf("空 platform 应归一化为 %s，got %s", PlatformAnthropic, got)
	}
	if got := NormalizeGroupPlatform(PlatformOpenAI); got != PlatformOpenAI {
		t.Fatalf("显式 platform 不应被改写，got %s", got)
	}
	// 归一化后的默认平台必须真的支持利润控制，否则预校验仍会误拒。
	if err := ValidateProfitControlConfig(NormalizeGroupPlatform(""), true, 0.3, 0.05); err != nil {
		t.Fatalf("省略 platform 且启用利润控制不应被预校验拒绝: %v", err)
	}
}
