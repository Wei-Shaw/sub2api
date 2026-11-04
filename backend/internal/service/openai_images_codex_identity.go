package service

import "net/http"

// Image requests share the verified local CLI version and User-Agent with Responses.
// Keep channel-specific header behavior, but never pin a separate client version.
const oauthImageCodexCLIVersion = codexCLIVersion
const oauthImageCodexCLIUserAgent = codexCLIUserAgent

func applyOAuthImageCodexIdentity(headers http.Header, account *Account) {
	if account == nil || account.Type != AccountTypeOAuth || account.UsesOpenAINativeImageChannel() || account.UsesChatGPTWebImageChannel() || account.IsOpenAIPersonalAccessToken() || account.IsOpenAIAgentIdentity() {
		return
	}
	headers.Set("User-Agent", oauthImageCodexCLIUserAgent)
	headers.Set("Originator", "codex_exec")
	headers.Set("Version", oauthImageCodexCLIVersion)
}
