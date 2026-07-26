package domain

import "strings"

// OpenAI agent-identity pure Account method + auth-mode const.
// Lifted from internal/service/openai_agent_identity.go in Phase 3. The
// impure task-registration / crypto path stays in service.

const OpenAIAuthModeAgentIdentity = "agentIdentity"

func (a *Account) IsOpenAIAgentIdentity() bool {
	if a == nil || !a.IsOpenAIOAuth() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.GetCredential(openAIAuthModeCredentialKey)), OpenAIAuthModeAgentIdentity)
}
