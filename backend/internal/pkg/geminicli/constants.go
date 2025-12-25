package geminicli

import "time"

const (
	AIStudioBaseURL  = "https://generativelanguage.googleapis.com"
	GeminiCliBaseURL = "https://cloudcode-pa.googleapis.com"

	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"

	// DefaultScopes is the minimal scope set for GeminiCli/CodeAssist usage.
	// Keep this conservative and expand only when we have a clear requirement.
	DefaultScopes = "https://www.googleapis.com/auth/cloud-platform"

	SessionTTL = 30 * time.Minute

	// GeminiCLIUserAgent mimics Gemini CLI to maximize compatibility with internal endpoints.
	GeminiCLIUserAgent = "GeminiCLI/0.1.5 (Windows; AMD64)"
)
