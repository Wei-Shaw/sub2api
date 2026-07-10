package service

import (
	"strings"
	"testing"
)

// D3-a 脱敏加固：身份证 / 银行卡 / 高熵 opaque token 应被精准脱敏。
// 直接测 redactGenerationStructuredPII（新增的唯一单元），与 email/phone、密钥脱敏隔离。

func TestRedactGenerationStructuredPII_CNIDCardRedacted(t *testing.T) {
	// 教科书合法身份证（年 1949、月 12、日 31，校验位 X）。
	const id = "11010519491231002X"
	out := redactGenerationStructuredPII("我的身份证是 " + id + " 请核对")
	if strings.Contains(out, id) {
		t.Errorf("身份证未脱敏: %s", out)
	}
	if !strings.Contains(out, "[ID]") {
		t.Errorf("缺少 [ID] 标记: %s", out)
	}
}

func TestRedactGenerationStructuredPII_BankCardRedacted(t *testing.T) {
	// 4111111111111111 是公开的 Visa 测试卡号（Luhn 合法）。
	for _, card := range []string{
		"4111111111111111",    // 连续
		"4111 1111 1111 1111", // 空格分组
		"4111-1111-1111-1111", // 短横线分组
	} {
		out := redactGenerationStructuredPII("卡号 " + card + " 谢谢")
		if strings.Contains(out, card) {
			t.Errorf("银行卡 %q 未脱敏: %s", card, out)
		}
		if !strings.Contains(out, "[CARD]") {
			t.Errorf("银行卡 %q 缺少 [CARD] 标记: %s", card, out)
		}
	}
}

func TestRedactGenerationStructuredPII_OpaqueTokenRedacted(t *testing.T) {
	// 24 位混合（含数字与字母）、无分隔、无已知前缀 → 现有 9 条正则漏网，应由 opaque pass 兜住。
	const tok = "Ab3Cd6Ef9Gh2Jk5Lm8Np1Qr4"
	out := redactGenerationStructuredPII("token=" + tok)
	if strings.Contains(out, tok) {
		t.Errorf("高熵 opaque token 未脱敏: %s", out)
	}
	if !strings.Contains(out, "[已脱敏]") {
		t.Errorf("缺少 [已脱敏] 标记: %s", out)
	}
}

func TestRedactGenerationStructuredPII_IdentifiersUntouched(t *testing.T) {
	// 这些是「正常标识符 / 业务内容」，必须原样保留（精准、不误抹）。
	ids := []string{
		"550e8400-e29b-41d4-a716-446655440000", // 标准 UUID（结构化 pass 不该碰，UUID 由 content_moderation 兜底）
		"1234567890123456",                     // 16 位非 Luhn 订单号
		"claude-opus-4-8",                      // 模型名
		"2024-01-15",                           // 日期
		"123456789012345678",                   // 18 位 snowflake（出生日期窗/校验位不过 → 非身份证；Luhn 不过 → 非卡）
		"12345678901234567890",                 // 20 位纯数字计数器
		"antidisestablishmentarianism",         // 28 位纯字母词（无数字 → 非 opaque 密钥）
	}
	for _, id := range ids {
		in := "标识符 " + id + " 结尾"
		out := redactGenerationStructuredPII(in)
		if out != in {
			t.Errorf("标识符 %q 被误抹: %q", id, out)
		}
	}
}

func TestRedactGenerationStructuredPII_InvalidIDNotRedacted(t *testing.T) {
	// 18 位、形似身份证但校验位错误 → 不应被当作身份证脱掉（精准）。
	const bad = "110105194912310021" // 末位改成 1，与正确校验位 X 不符
	in := "号码 " + bad + " 备注"
	out := redactGenerationStructuredPII(in)
	if out != in {
		t.Errorf("校验位错误的伪身份证被误抹: %q", out)
	}
}

func TestRedactGenerationStructuredPII_NormalChinesePreserved(t *testing.T) {
	// 大段正常中文工单 prose：结构化 pass 不得碰（样本墙要展示真实生产内容）。
	const prose = "用户反馈登录后首页加载很慢，希望排查接口超时问题，并给出优化建议和预计修复时间。"
	if out := redactGenerationStructuredPII(prose); out != prose {
		t.Errorf("正常中文 prose 被误抹: %q", out)
	}
}

// 端到端：身份证 + 银行卡过完整 prompt 管线应被脱掉，正常中文与模型名保留。
func TestRedactGenerationPrompt_IDAndCardRedacted(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","messages":[{"role":"user","content":"身份证11010519491231002X 卡号4111111111111111 请帮我核对账户"}]}`)
	out := redactGenerationPrompt(body)
	if strings.Contains(out, "11010519491231002X") {
		t.Errorf("prompt 管线漏掉身份证: %s", out)
	}
	if strings.Contains(out, "4111111111111111") {
		t.Errorf("prompt 管线漏掉银行卡: %s", out)
	}
	if !strings.Contains(out, "claude-opus-4-8") {
		t.Errorf("模型名被误抹: %s", out)
	}
	if !strings.Contains(out, "请帮我核对账户") {
		t.Errorf("正常中文被误抹: %s", out)
	}
}
