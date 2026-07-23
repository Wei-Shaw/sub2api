package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// isChatGPTWorkspaceAccountMismatch identifies the ChatGPT upstream response
// emitted when an OAuth token is paired with a stale or unrelated workspace
// id. The account can still be valid without the optional workspace header, so
// callers may safely retry once after removing chatgpt-account-id.
func isChatGPTWorkspaceAccountMismatch(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusConflict || len(responseBody) == 0 {
		return false
	}
	lower := strings.ToLower(string(responseBody))
	return strings.Contains(lower, "sa_server_user_does_not_belong_to_workspace") ||
		(strings.Contains(lower, "does not belong to") && strings.Contains(lower, "workspace"))
}

// blockChatGPTWorkspaceMismatch keeps an OAuth account whose workspace
// metadata is incompatible with its token out of the scheduler briefly. The
// persisted credentials are intentionally left untouched because the account
// may still be valid for a different workspace after an operator refresh.
func (s *OpenAIGatewayService) blockChatGPTWorkspaceMismatch(account *Account) {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || !account.IsOpenAIOAuth() {
		return
	}
	s.BlockAccountScheduling(account, time.Time{}, "workspace_mismatch")
}

func setOpenAIChatGPTAccountHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
		headers.Set("chatgpt-account-id", chatgptAccountID)
	}
	if account.IsChatGPTAccountFedRAMP() {
		headers.Set("x-openai-fedramp", "true")
	} else {
		headers.Del("x-openai-fedramp")
	}
}

// resolveAndSetOpenAIChatGPTAccountHeaders 解析 spark 影子账号至其母账号（凭据透传），
// 再调用 setOpenAIChatGPTAccountHeaders 写入 chatgpt-account-id / x-openai-fedramp 头。
// 普通账号（非影子）为直通，行为与直接调用 setOpenAIChatGPTAccountHeaders 一致。
func resolveAndSetOpenAIChatGPTAccountHeaders(ctx context.Context, repo AccountRepository, headers http.Header, account *Account) error {
	credAccount, err := resolveCredentialAccount(ctx, repo, account)
	if err != nil {
		return err
	}
	setOpenAIChatGPTAccountHeaders(headers, credAccount)
	return nil
}
