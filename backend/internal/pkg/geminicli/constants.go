package geminicli

import "time"

const (
	AIStudioBaseURL  = "https://generativelanguage.googleapis.com"
	GeminiCliBaseURL = "https://cloudcode-pa.googleapis.com"

	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"

	// AIStudioOAuthRedirectURI is the default redirect URI used for AI Studio OAuth.
	// This matches the "copy/paste callback URL" flow used by OpenAI OAuth in this project.
	// Note: You still need to register this redirect URI in your Google OAuth client
	// unless you use an OAuth client type that permits localhost redirect URIs.
	AIStudioOAuthRedirectURI = "http://localhost:1455/auth/callback"

	// DefaultScopes for Code Assist (includes cloud-platform for API access plus userinfo scopes)
	// Required by Google's Code Assist API.
	DefaultCodeAssistScopes = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"

	// DefaultScopes for AI Studio (uses generativelanguage API with OAuth)
	// Reference: https://ai.google.dev/gemini-api/docs/oauth
	// For regular Google accounts, supports API calls to generativelanguage.googleapis.com
	DefaultAIStudioScopes = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/generative-language"

	// GeminiCLIRedirectURI is the redirect URI used by Gemini CLI for Code Assist OAuth.
	GeminiCLIRedirectURI = "https://codeassist.google.com/authcode"

	SessionTTL = 30 * time.Minute

	// GeminiCLIUserAgent mimics Gemini CLI to maximize compatibility with internal endpoints.
	GeminiCLIUserAgent = "GeminiCLI/0.1.5 (Windows; AMD64)"
)
