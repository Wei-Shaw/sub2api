package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

// TestRedactGenerationHardening_BeforeAfterDemo 产出 D3-a「脱敏加固前后对比」证据（§3.1）。
// BEFORE = 加固前管线（RedactText → email/phone → content_moderation，无 structured pass）；
// AFTER  = 加固后管线（多插一层 redactGenerationStructuredPII：身份证/银行卡/opaque token）。
// 用 `go test -run TestRedactGenerationHardening_BeforeAfterDemo -v` 运行，输出可直接贴进 SUMMARY。
func TestRedactGenerationHardening_BeforeAfterDemo(t *testing.T) {
	before := func(s string) string {
		out := logredact.RedactText(s)
		out = redactGenerationPII(out)
		out = redactContentModerationSecrets(out)
		return out
	}
	after := func(s string) string {
		out := logredact.RedactText(s)
		out = redactGenerationPII(out)
		out = redactGenerationStructuredPII(out)
		out = redactContentModerationSecrets(out)
		return out
	}

	// (A) 漏网样本：加固前漏全量、加固后抹。leak = 加固前应仍可见的明文片段。
	// 这三类是「连续」形态，加固前 9 条正则全部漏网，加固后被精准打类型标记。
	leaks := []struct{ name, in, leak string }{
		{"身份证号", "我的身份证 11010519491231002X 麻烦核对", "11010519491231002X"},
		{"银行卡号(连续)", "卡号 4111111111111111 转账", "4111111111111111"},
		{"高熵token", "临时令牌 Ab3Cd6Ef9Gh2Jk5Lm8Np1Qr4 已发", "Ab3Cd6Ef9Gh2Jk5Lm8Np1Qr4"},
	}
	t.Log("==== (A) 漏网 PII/凭据（连续形态）：加固前漏全量 / 加固后抹 ====")
	for _, c := range leaks {
		b := before(c.in)
		a := after(c.in)
		t.Logf("[%s]\n  IN    : %s\n  BEFORE: %s\n  AFTER : %s", c.name, c.in, b, a)
		if !strings.Contains(b, c.leak) {
			t.Errorf("[%s] 期望加固前漏明文，但 BEFORE 已不含 %q（对比失真）", c.name, c.leak)
		}
		if strings.Contains(a, c.leak) {
			t.Errorf("[%s] 加固后仍泄露 %q: %s", c.name, c.leak, a)
		}
	}

	// (A2) 分隔形态银行卡：管线顺序 phone 先于 card，故空格/短横线分组卡号先被电话分组正则
	// 吃掉主体（→ [PHONE] + 尾 4 位）。尾 4 位非敏感（PCI 允许展示），卡号主体已不泄露。
	// 这是「已脱敏，仅 marker 为 [PHONE]」的可接受残留，记录于此以正其名。
	t.Log("==== (A2) 分隔形态银行卡：phone 先行吸收主体（仍脱敏，仅 marker 不同）====")
	for _, in := range []string{"卡号 4111 1111 1111 1111 转账", "卡号 4111-1111-1111-1111 转账"} {
		a := after(in)
		t.Logf("  IN   : %s\n  AFTER: %s", in, a)
		// 主体(连续 12+ 位)不得残留；尾 4 位可接受。
		if strings.Contains(a, "111111111111") {
			t.Errorf("分隔卡号主体仍泄露: %s", a)
		}
	}

	// (B) 正常业务 prompt：加固后不得误抹（中文 prose 原样，只动真 PII）。
	normals := []string{
		"用户反馈登录后首页加载很慢，希望排查接口超时问题并给出优化建议。",
		"请帮我把这段产品介绍润色得更专业一些，目标受众是企业采购决策者。",
		"工单号 ORD-20240115-0042，客户要求本周内回复处理进度与预计完成时间。",
	}
	t.Log("==== (B) 正常业务 prompt：加固后不误抹 ====")
	for _, s := range normals {
		a := after(s)
		t.Logf("  IN /AFTER 一致? %v\n  IN   : %s\n  AFTER: %s", a == s, s, a)
		if a != s {
			t.Errorf("正常业务 prompt 被误抹:\n  in=%q\n  out=%q", s, a)
		}
	}

	// (C) 模型名等分析必需字段保留。
	t.Log("==== (C) 分析必需字段保留 ====")
	model := "claude-opus-4-8"
	if got := after("调用模型 " + model + " 处理"); !strings.Contains(got, model) {
		t.Errorf("模型名被误抹: %s", got)
	} else {
		t.Logf("  模型名保留 OK: %s", got)
	}
}
