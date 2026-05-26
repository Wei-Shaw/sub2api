// Package pluginsdk provides shared account type identifiers used by all
// gateway plugins so the host and plugins agree on the canonical value
// stored in the accounts.type column.
package pluginsdk

const (
	// AccountTypeOAuth identifies accounts authenticated via OAuth flow.
	AccountTypeOAuth = "oauth"
	// AccountTypeAPIKey identifies accounts authenticated via API key.
	AccountTypeAPIKey = "apikey"
	// AccountTypeSetupToken identifies accounts using a long-lived setup
	// token (Anthropic / OpenAI Codex flow).
	AccountTypeSetupToken = "setup-token"
	// AccountTypeUpstream identifies upstream-proxy accounts that forward
	// traffic to another sub2api instance.
	AccountTypeUpstream = "upstream"
	// AccountTypeServiceAccount identifies Google service-account credentials
	// used by Vertex AI (Gemini / Anthropic Vertex).
	AccountTypeServiceAccount = "service_account"
	// AccountTypeBedrock identifies AWS Bedrock-backed Anthropic accounts.
	AccountTypeBedrock = "bedrock"
)
