package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// codexFingerprintCandidate 描述一种真实 Codex 客户端可能出现的运行环境：Codex CLI（codex-rs
// 引擎）通过 os_info crate 探测运行系统，出站 User-Agent 里的 (OS;架构) 终端片段会反映真实
// 运行环境，而不是固定值。候选项内容依据对 @openai/codex 真实二进制的静态分析结论（见
// specs/001-codex-ua-diversity/research.md §3），只收录真实存在的 OS/终端组合。
type codexFingerprintCandidate struct {
	os       string // 如 "macOS 15.1.0"
	arch     string // 如 "arm64"
	terminal string // 如 "iTerm.app"
}

// codexFingerprintPool 是内置的指纹候选池，供未手填 User-Agent 的 OpenAI OAuth 账号确定性
// 分配使用。覆盖 macOS / Windows / 主流 Linux 发行版各自常见的真实终端组合，避免所有账号出站
// 呈现同一种 (OS;架构)+终端，成为可被批量识别的异常信号。
var codexFingerprintPool = []codexFingerprintCandidate{
	{os: "Ubuntu 22.4.0", arch: "x86_64", terminal: "xterm-256color"},
	{os: "Ubuntu 24.4.0", arch: "x86_64", terminal: "tmux-256color"},
	{os: "Debian GNU/Linux 12", arch: "x86_64", terminal: "gnome-terminal"},
	{os: "Fedora Linux 40", arch: "x86_64", terminal: "xterm-256color"},
	{os: "macOS 14.5.0", arch: "arm64", terminal: "iTerm.app"},
	{os: "macOS 15.1.0", arch: "arm64", terminal: "Apple_Terminal"},
	{os: "Windows 11", arch: "x86_64", terminal: "conhost"},
}

// selectCodexFingerprint 按账号 ID 从候选池确定性选取一项：同一账号 ID 多次调用恒返回相同
// 结果（不逐请求随机跳变），不同账号 ID 在池长度 >= 2 时倾向选出不同结果。纯函数，无副作用。
// 候选池为空时返回 (零值, false)，调用方需自行处理"不可用"分支，不会 panic。
func selectCodexFingerprint(pool []codexFingerprintCandidate, accountID int64) (codexFingerprintCandidate, bool) {
	if len(pool) == 0 {
		return codexFingerprintCandidate{}, false
	}
	idx := uint64(accountID) % uint64(len(pool))
	return pool[idx], true
}

// buildCodexFingerprintUserAgent 把候选项拼成完整的 Codex 形态 User-Agent 字符串：
// "{originator}/{version} ({os}; {arch}) {terminal}"。只产出 UA 字符串本身，不涉及
// originator/version 的出站身份重建——那部分统一交给 resolveCodexOutboundIdentity /
// PairCodexClientIdentity 收口（宪法原则 I），本函数的输出只是它们的输入候选之一，version
// 段会在后续被 resolveCodexOutboundIdentity 按当前生效版本重写，此处填什么值不影响最终出站。
func buildCodexFingerprintUserAgent(c codexFingerprintCandidate) string {
	return fmt.Sprintf("%s/%s (%s; %s) %s", openai.CodexDefaultOriginator, codexCLIVersion, c.os, c.arch, c.terminal)
}

// assignCodexFingerprintPoolUserAgent 只在新建（已通过 accountRepo.Create 拿到数据库自增 ID）
// 的 OpenAI OAuth 账号且未手填 user_agent 凭据时生效：从候选池按账号 ID 分配一项、拼出 UA
// 写入凭据并落库一次。候选池不可用（返回 ok=false）或账号已手填 user_agent 时直接跳过，不报
// 错、不落库——分配失败不应阻断账号创建（FR-006）。只应在 CreateAccount 里调用一次；编辑既有
// 账号不会触发本函数，避免存量账号被意外补上分配结果（FR-007）。
func (s *adminServiceImpl) assignCodexFingerprintPoolUserAgent(ctx context.Context, account *Account) error {
	if !account.IsOpenAIOAuth() || account.GetOpenAIUserAgent() != "" {
		return nil
	}
	candidate, ok := selectCodexFingerprint(codexFingerprintPool, account.ID)
	if !ok {
		return nil
	}
	if account.Credentials == nil {
		account.Credentials = make(map[string]any, 1)
	}
	account.Credentials["user_agent"] = buildCodexFingerprintUserAgent(candidate)
	return s.accountRepo.Update(ctx, account)
}
