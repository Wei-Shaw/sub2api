package claude

import (
	"net/http"
	"strings"
)

// MimicMode controls how a non-Claude client is represented on an OAuth
// request. Compatibility preserves the historical system-to-messages bridge;
// strict keeps the wire shape closer to Claude Code's own system payload.
type MimicMode string

const (
	MimicModeCompatibility MimicMode = "compatibility"
	MimicModeStrict        MimicMode = "strict"
)

// Profile is the versioned identity contract used by Claude Code mimicry.
// Keep protocol data here so a CLI upgrade is one profile change instead of a
// collection of unrelated constants in request builders.
type Profile struct {
	ID              string
	Version         string
	Entrypoint      string
	UserAgent       string
	SystemPrompt    string
	ExpansionPrompt string
	CacheControlTTL string
	Betas           []string
	Headers         map[string]string
}

const (
	ProfileClaudeCodeCLI     = "claude-code-cli"
	ProfileClaudeAgentSDK    = "claude-agent-sdk"
	ProfileClaudeCodeIDE     = "claude-code-ide"
	DefaultProfileID         = ProfileClaudeCodeCLI
	DefaultProfileVersion    = CLICurrentVersion
	defaultProfileEntrypoint = "cli"
	defaultCacheControlTTL   = "1h"
)

var profileHeaders = map[string]string{
	"X-Stainless-Lang":                          "js",
	"X-Stainless-Package-Version":               "0.94.0",
	"X-Stainless-OS":                            "Linux",
	"X-Stainless-Arch":                          "arm64",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v24.3.0",
	"X-Stainless-Retry-Count":                   "0",
	"X-Stainless-Timeout":                       "600",
	"X-App":                                     "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
}

var profiles = map[string]Profile{
	ProfileClaudeCodeCLI: {
		ID:              ProfileClaudeCodeCLI,
		Version:         CLICurrentVersion,
		Entrypoint:      defaultProfileEntrypoint,
		UserAgent:       "claude-cli/" + CLICurrentVersion + " (external, cli)",
		SystemPrompt:    "You are Claude Code, Anthropic's official CLI for Claude.",
		CacheControlTTL: defaultCacheControlTTL,
		Betas:           FullClaudeCodeMimicryBetas(),
		Headers:         profileHeaders,
	},
	ProfileClaudeAgentSDK: {
		ID:              ProfileClaudeAgentSDK,
		Version:         CLICurrentVersion,
		Entrypoint:      "sdk",
		UserAgent:       "claude-cli/" + CLICurrentVersion + " (external, sdk, agent-sdk/" + CLICurrentVersion + ")",
		SystemPrompt:    "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.",
		CacheControlTTL: defaultCacheControlTTL,
		Betas:           FullClaudeCodeMimicryBetas(),
		Headers:         profileHeaders,
	},
	ProfileClaudeCodeIDE: {
		ID:              ProfileClaudeCodeIDE,
		Version:         CLICurrentVersion,
		Entrypoint:      "claude-vscode",
		UserAgent:       "claude-cli/" + CLICurrentVersion + " (external, claude-vscode)",
		SystemPrompt:    "You are Claude Code, Anthropic's official CLI for Claude.",
		CacheControlTTL: defaultCacheControlTTL,
		Betas:           FullClaudeCodeMimicryBetas(),
		Headers:         profileHeaders,
	},
}

// ResolveProfile returns a defensive copy. Unknown/empty IDs intentionally
// fall back to the CLI profile so a bad deployment setting cannot emit a
// partially populated identity.
func ResolveProfile(id string) Profile {
	id = strings.ToLower(strings.TrimSpace(id))
	profile, ok := profiles[id]
	if !ok {
		profile = profiles[DefaultProfileID]
	}
	profile.Betas = append([]string(nil), profile.Betas...)
	profile.Headers = cloneHeaders(profile.Headers)
	if profile.CacheControlTTL == "" {
		profile.CacheControlTTL = defaultCacheControlTTL
	}
	return profile
}

func cloneHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// HeadersForRequest returns profile headers with User-Agent and Accept filled
// in. It is intentionally independent from http.Request for Golden tests.
func HeadersForRequest(profile Profile) http.Header {
	out := make(http.Header, len(profile.Headers)+2)
	for key, value := range profile.Headers {
		if strings.TrimSpace(value) != "" {
			out.Set(key, value)
		}
	}
	if profile.UserAgent != "" {
		out.Set("User-Agent", profile.UserAgent)
	}
	out.Set("Accept", "application/json")
	return out
}

func NormalizeMimicMode(raw string) MimicMode {
	if strings.EqualFold(strings.TrimSpace(raw), string(MimicModeStrict)) {
		return MimicModeStrict
	}
	return MimicModeCompatibility
}
