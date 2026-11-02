package service

import "testing"

func TestOpenAIDecisionsCapabilityOnlySelectsAPIKeyAndUpstream(t *testing.T) {
	for name, account := range map[string]*Account{
		"api key":  {Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		"upstream": {Platform: PlatformOpenAI, Type: AccountTypeUpstream},
		"oauth":    {Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		"grok":     {Platform: PlatformGrok, Type: AccountTypeAPIKey},
	} {
		want := name == "api key" || name == "upstream"
		if got := account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions); got != want {
			t.Errorf("%s: decisions capability = %t, want %t", name, got, want)
		}
	}
}

func TestOpenAIDecisionsCapabilityHonorsExplicitList(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"openai_capabilities": []any{"chat_completions"},
	}}
	if account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions) {
		t.Fatal("explicit capability list without decisions should exclude the endpoint")
	}
	account.Credentials["openai_capabilities"] = []any{"chat_completions", "decisions"}
	if !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions) {
		t.Fatal("explicit capability list with decisions should include the endpoint")
	}
}
