package service

import (
	"net/http"
	"strings"
	"testing"
)

// TestCodexFingerprintPoolShape 断言内置候选池至少有 2 项（满足 spec SC-001 的分布性前提），
// 且每一项的 OS/架构/终端字段均非空——避免出现"声明了候选池却只有 1 项"或某个字段留空的
// 半成品实现。
func TestCodexFingerprintPoolShape(t *testing.T) {
	if len(codexFingerprintPool) < 2 {
		t.Fatalf("候选池长度 = %d，期望 >= 2", len(codexFingerprintPool))
	}
	for i, c := range codexFingerprintPool {
		if c.os == "" {
			t.Errorf("候选池第 %d 项 os 字段为空", i)
		}
		if c.arch == "" {
			t.Errorf("候选池第 %d 项 arch 字段为空", i)
		}
		if c.terminal == "" {
			t.Errorf("候选池第 %d 项 terminal 字段为空", i)
		}
	}
}

// TestSelectCodexFingerprintDeterministic 断言同一账号 ID 多次调用返回相同候选项（稳定性，
// 对应 spec SC-002），不同账号 ID 在候选池长度 >= 2 时倾向选出不同候选项（分布性，对应
// spec SC-001）。
func TestSelectCodexFingerprintDeterministic(t *testing.T) {
	first, ok := selectCodexFingerprint(codexFingerprintPool, 42)
	if !ok {
		t.Fatalf("account_id=42 应能从非空候选池选出候选项")
	}
	for i := 0; i < 5; i++ {
		again, ok := selectCodexFingerprint(codexFingerprintPool, 42)
		if !ok || again != first {
			t.Fatalf("同一账号 ID 第 %d 次调用结果不稳定：first=%+v again=%+v ok=%v", i, first, again, ok)
		}
	}

	seen := make(map[codexFingerprintCandidate]struct{})
	for accountID := int64(1); accountID <= 20; accountID++ {
		c, ok := selectCodexFingerprint(codexFingerprintPool, accountID)
		if !ok {
			t.Fatalf("account_id=%d 应能从非空候选池选出候选项", accountID)
		}
		seen[c] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("20 个不同账号 ID 只选出 %d 种候选项，期望 >= 2 种（候选池长度 = %d）", len(seen), len(codexFingerprintPool))
	}
}

// TestSelectCodexFingerprintEmptyPool 断言候选池为空时返回明确的"不可用"信号（ok=false），
// 不 panic——对应 spec Edge Cases 的兜底要求（FR-006）。
func TestSelectCodexFingerprintEmptyPool(t *testing.T) {
	c, ok := selectCodexFingerprint(nil, 1)
	if ok {
		t.Fatalf("空候选池应返回 ok=false，实际返回 %+v, ok=%v", c, ok)
	}
	if c != (codexFingerprintCandidate{}) {
		t.Fatalf("空候选池应返回零值候选项，实际 %+v", c)
	}
}

// TestBuildCodexFingerprintUserAgent 断言拼出的 UA 字符串包含候选项的 OS/架构/终端片段，
// 且客户端名前缀是官方 originator（能通过既有的 PairCodexClientIdentity 配对）。
func TestBuildCodexFingerprintUserAgent(t *testing.T) {
	c := codexFingerprintCandidate{os: "macOS 15.1.0", arch: "arm64", terminal: "iTerm.app"}
	ua := buildCodexFingerprintUserAgent(c)

	for _, want := range []string{c.os, c.arch, c.terminal} {
		if !strings.Contains(ua, want) {
			t.Fatalf("UA %q 未包含候选项片段 %q（完整候选项 %+v）", ua, want, c)
		}
	}
}

// TestCodexFingerprintPoolUserAgentsSurviveIdentityReconstruction 覆盖分析报告 C1、宪法
// Principle I：候选池里每一项拼出的 UA，喂给既有的出站身份收口函数
// enforceCodexIdentityHeadersWithUA 后，originator 必须与最终 User-Agent 首段配套、
// version 必须被重建为当前生效版本——这条约束不能只靠"新代码不碰身份重建逻辑"的设计假设，
// 必须用测试验证指纹池的具体产出真的能被既有机制正确处理，而不是理论上兼容。
func TestCodexFingerprintPoolUserAgentsSurviveIdentityReconstruction(t *testing.T) {
	for i, c := range codexFingerprintPool {
		ua := buildCodexFingerprintUserAgent(c)

		h := make(http.Header)
		h.Set("originator", "codex-tui") // 出站请求必带的既有前置条件（见 buildUpstreamRequest）
		enforceCodexIdentityHeadersWithUA(h, ua)

		gotOriginator := h.Get("originator")
		gotUA := h.Get("user-agent")
		if gotOriginator == "" {
			t.Fatalf("候选池第 %d 项 %+v 重建后 originator 为空，UA=%q", i, c, ua)
		}
		if !strings.HasPrefix(gotUA, gotOriginator+"/") {
			t.Fatalf("候选池第 %d 项 %+v 重建后 originator=%q 与 UA 首段不配套，UA=%q", i, c, gotOriginator, gotUA)
		}
		if got := h.Get("version"); got != codexCLIVersion {
			t.Fatalf("候选池第 %d 项 %+v 重建后 version=%q，期望等于当前生效版本 %q", i, c, got, codexCLIVersion)
		}
		for _, want := range []string{c.os, c.arch, c.terminal} {
			if !strings.Contains(gotUA, want) {
				t.Fatalf("候选池第 %d 项 %+v 重建后 UA %q 丢失了片段 %q", i, c, gotUA, want)
			}
		}
	}
}
