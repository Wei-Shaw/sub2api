package service

import (
	"context"
	"net/http"
)

// resolveOpenAIForceCodexIdentityEnabled 解析账号到有效母账号（spark 影子继承母账号开关，
// 不在影子自身 extra 复制），返回 force_codex_identity 状态。
func resolveOpenAIForceCodexIdentityEnabled(ctx context.Context, repo AccountRepository, account *Account) (bool, error) {
	credAccount, err := resolveCredentialAccount(ctx, repo, account)
	if err != nil {
		return false, err
	}
	return credAccount.IsForceCodexIdentityEnabled(), nil
}

// applyForcedCodexCLIUserAgent 将最终出站 UA 固定为 Codex CLI。originator 与 version
// 继续由紧随其后的 enforceCodexIdentityHeaders 统一配对和校正。
func applyForcedCodexCLIUserAgent(header http.Header) {
	if header != nil {
		header.Set("user-agent", codexCLIUserAgent)
	}
}
