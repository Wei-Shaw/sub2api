package service

import openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

const openAIOAuthDefaultTestModel = "gpt-5.4"

// defaultOpenAITestModelForAccount chooses a safe probe model for the account.
// ChatGPT OAuth accounts on chatgpt.com/backend-api/codex/responses reject
// gpt-5.1-codex and gpt-5.1, but accept gpt-5.4 in the live runtime.
func defaultOpenAITestModelForAccount(account *Account) string {
	if account != nil && account.IsOpenAIOAuth() {
		return openAIOAuthDefaultTestModel
	}
	return openaipkg.DefaultTestModel
}
