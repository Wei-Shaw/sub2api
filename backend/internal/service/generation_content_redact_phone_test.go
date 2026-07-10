package service

import (
	"strings"
	"testing"
)

// 收窄后的电话正则：常见电话被脱为 [PHONE]，但标准 UUID / 长纯数字 request-id / 模型名不被误伤。
// 直接测 redactGenerationPII（脱敏管线中唯一改动的单元），与密钥脱敏隔离开。
func TestRedactGenerationPII_PhonesRedacted(t *testing.T) {
	phones := []string{
		"+1 415 555 0199",   // 国际 + 空格分隔
		"+86 139 1234 5678", // 国际带区号
		"13912345678",       // 中国手机（纯 11 位）
		"010-1234-5678",     // 短横线分隔分组
	}
	for _, ph := range phones {
		out := redactGenerationPII("contact me at " + ph + " thanks")
		if strings.Contains(out, ph) {
			t.Errorf("phone %q leaked: %s", ph, out)
		}
		if !strings.Contains(out, "[PHONE]") {
			t.Errorf("phone %q: expected [PHONE] marker, got: %s", ph, out)
		}
	}
}

func TestRedactGenerationPII_IdentifiersUntouched(t *testing.T) {
	// 这些是「不该当电话脱掉」的标识符（B.1 Codex 复核标黄的误伤项）。
	ids := []string{
		"550e8400-e29b-41d4-a716-446655440000", // 标准 UUID（末段 12 位纯数字——旧正则会把它当电话）
		"1234567890123456",                     // 16 位纯数字订单/请求 id
		"claude-opus-4-8",                      // 模型名（分析需要保留）
		"2024-01-15",                           // 日期，不应被分组正则命中
	}
	for _, id := range ids {
		in := "identifier " + id + " trailing"
		out := redactGenerationPII(in)
		if out != in {
			t.Errorf("identifier %q was modified by email/phone pass: %q", id, out)
		}
	}
}

// 端到端兜底：UUID 过完整管线后应被「密钥脱敏」整体脱掉，而不是被电话正则切碎成残片。
func TestRedactGenerationPrompt_UUIDNotMangledByPhone(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"trace id 550e8400-e29b-41d4-a716-446655440000 done"}]}`)
	out := redactGenerationPrompt(body)
	if strings.Contains(out, "[PHONE]") {
		t.Errorf("UUID must not be turned into [PHONE]: %s", out)
	}
	if strings.Contains(out, "446655440000") {
		t.Errorf("UUID tail leaked (phone regex mangled it): %s", out)
	}
}
