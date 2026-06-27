package service

import (
	"encoding/json"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PlatformProfileRegistry is the long-term registry for provider/platform guide metadata.
// It is stored as one JSON setting so operators can insert/edit guide copy without
// hardcoding platform-specific instructions into the frontend.
type PlatformProfileRegistry struct {
	Version  int               `json:"version"`
	Profiles []PlatformProfile `json:"profiles"`
}

// PlatformProfile describes one platform/provider profile.
type PlatformProfile struct {
	Platform     string                `json:"platform"`
	ProviderID   string                `json:"provider_id,omitempty"`
	ProviderName string                `json:"provider_name,omitempty"`
	APIStyle     string                `json:"api_style,omitempty"`
	Guide        PlatformGuideMetadata `json:"guide"`
}

// PlatformGuideMetadata is safe to expose publicly and through provider-catalog.
type PlatformGuideMetadata struct {
	ProfileID     string                   `json:"profile_id"`
	Title         string                   `json:"title"`
	Description   string                   `json:"description"`
	Note          string                   `json:"note,omitempty"`
	DocsURL       string                   `json:"docs_url,omitempty"`
	DefaultClient string                   `json:"default_client,omitempty"`
	Clients       []PlatformGuideClient    `json:"clients,omitempty"`
	CopyBlocks    []PlatformGuideCopyBlock `json:"copy_blocks,omitempty"`
}

// PlatformGuideClient describes a user-facing client tab/target.
type PlatformGuideClient struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	OS    []string `json:"os,omitempty"`
}

// PlatformGuideCopyBlock is a templated copyable guide block. The frontend replaces
// placeholders such as {{base_url}}, {{api_base_url}}, and {{api_key}} at render time.
type PlatformGuideCopyBlock struct {
	ID              string `json:"id"`
	ClientID        string `json:"client_id"`
	OS              string `json:"os,omitempty"`
	Path            string `json:"path"`
	Hint            string `json:"hint,omitempty"`
	Language        string `json:"language,omitempty"`
	ContentTemplate string `json:"content_template"`
}

func defaultPlatformProfileRegistry() PlatformProfileRegistry {
	return PlatformProfileRegistry{
		Version: 1,
		Profiles: []PlatformProfile{
			{
				Platform:     "openai",
				ProviderID:   "v-claw-openai",
				ProviderName: "OpenAI",
				APIStyle:     "openai-responses",
				Guide: PlatformGuideMetadata{
					ProfileID:     "openai",
					Title:         "OpenAI / Codex guide",
					Description:   "Use this API key with OpenAI-compatible clients and Codex. The values below are generated from the platform profile registry, not hardcoded per modal.",
					Note:          "For Codex, keep response storage disabled and point the provider base URL to your V-Claw endpoint.",
					DocsURL:       "https://platform.openai.com/docs",
					DefaultClient: "codex",
					Clients: []PlatformGuideClient{
						{ID: "codex", Label: "Codex CLI", OS: []string{"unix", "windows"}},
						{ID: "codex-ws", Label: "Codex CLI WebSocket", OS: []string{"unix", "windows"}},
						{ID: "claude", Label: "Claude Code compatibility", OS: []string{"unix", "cmd", "powershell"}},
					},
					CopyBlocks: []PlatformGuideCopyBlock{
						{ID: "openai-codex-config-unix", ClientID: "codex", OS: "unix", Path: "~/.codex/config.toml", Hint: "Codex config.toml", Language: "toml", ContentTemplate: `model_provider = "OpenAI"
model = "{{openai_model}}"
review_model = "{{openai_model}}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "{{base_url}}"
wire_api = "responses"
requires_openai_auth = true

[features]
goals = true`},
						{ID: "openai-codex-auth-unix", ClientID: "codex", OS: "unix", Path: "~/.codex/auth.json", Language: "json", ContentTemplate: `{
  "OPENAI_API_KEY": "{{api_key}}"
}`},
						{ID: "openai-codex-config-windows", ClientID: "codex", OS: "windows", Path: "%userprofile%\\.codex\\config.toml", Hint: "Codex config.toml", Language: "toml", ContentTemplate: `model_provider = "OpenAI"
model = "{{openai_model}}"
review_model = "{{openai_model}}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "{{base_url}}"
wire_api = "responses"
requires_openai_auth = true

[features]
goals = true`},
						{ID: "openai-codex-auth-windows", ClientID: "codex", OS: "windows", Path: "%userprofile%\\.codex\\auth.json", Language: "json", ContentTemplate: `{
  "OPENAI_API_KEY": "{{api_key}}"
}`},
						{ID: "openai-codex-ws-config-unix", ClientID: "codex-ws", OS: "unix", Path: "~/.codex/config.toml", Hint: "Codex WebSocket config.toml", Language: "toml", ContentTemplate: `model_provider = "OpenAI"
model = "{{openai_model}}"
review_model = "{{openai_model}}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "{{base_url}}"
wire_api = "responses"
supports_websockets = true
requires_openai_auth = true

[features]
responses_websockets_v2 = true
goals = true`},
						{ID: "openai-codex-ws-auth-unix", ClientID: "codex-ws", OS: "unix", Path: "~/.codex/auth.json", Language: "json", ContentTemplate: `{
  "OPENAI_API_KEY": "{{api_key}}"
}`},
						{ID: "openai-codex-ws-config-windows", ClientID: "codex-ws", OS: "windows", Path: "%userprofile%\\.codex\\config.toml", Hint: "Codex WebSocket config.toml", Language: "toml", ContentTemplate: `model_provider = "OpenAI"
model = "{{openai_model}}"
review_model = "{{openai_model}}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "{{base_url}}"
wire_api = "responses"
supports_websockets = true
requires_openai_auth = true

[features]
responses_websockets_v2 = true
goals = true`},
						{ID: "openai-codex-ws-auth-windows", ClientID: "codex-ws", OS: "windows", Path: "%userprofile%\\.codex\\auth.json", Language: "json", ContentTemplate: `{
  "OPENAI_API_KEY": "{{api_key}}"
}`},
					},
				},
			},
			{
				Platform:     "anthropic",
				ProviderID:   "v-claw-anthropic",
				ProviderName: "Anthropic",
				APIStyle:     "anthropic-messages",
				Guide: PlatformGuideMetadata{
					ProfileID:     "anthropic",
					Title:         "Anthropic / Claude Code guide",
					Description:   "Use this API key as an Anthropic-compatible token for Claude Code and compatible tools.",
					Note:          "Claude Code reads ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN from the shell or ~/.claude/settings.json.",
					DocsURL:       "https://docs.anthropic.com/claude/docs/claude-code",
					DefaultClient: "claude",
					Clients: []PlatformGuideClient{
						{ID: "claude", Label: "Claude Code", OS: []string{"unix", "cmd", "powershell"}},
						{ID: "opencode", Label: "OpenCode"},
					},
					CopyBlocks: []PlatformGuideCopyBlock{
						{ID: "anthropic-env-unix", ClientID: "claude", OS: "unix", Path: "Terminal", Language: "shell", ContentTemplate: `export ANTHROPIC_BASE_URL="{{base_url}}"
export ANTHROPIC_AUTH_TOKEN="{{api_key}}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`},
						{ID: "anthropic-env-cmd", ClientID: "claude", OS: "cmd", Path: "Command Prompt", Language: "bat", ContentTemplate: `set ANTHROPIC_BASE_URL={{base_url}}
set ANTHROPIC_AUTH_TOKEN={{api_key}}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`},
						{ID: "anthropic-env-powershell", ClientID: "claude", OS: "powershell", Path: "PowerShell", Language: "powershell", ContentTemplate: `$env:ANTHROPIC_BASE_URL="{{base_url}}"
$env:ANTHROPIC_AUTH_TOKEN="{{api_key}}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`},
						{ID: "anthropic-claude-settings-unix", ClientID: "claude", OS: "unix", Path: "~/.claude/settings.json", Hint: "VSCode Claude Code", Language: "json", ContentTemplate: `{
  "env": {
    "ANTHROPIC_BASE_URL": "{{base_url}}",
    "ANTHROPIC_AUTH_TOKEN": "{{api_key}}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`},
						{ID: "anthropic-claude-settings-windows", ClientID: "claude", OS: "cmd", Path: "%userprofile%\\.claude\\settings.json", Hint: "VSCode Claude Code", Language: "json", ContentTemplate: `{
  "env": {
    "ANTHROPIC_BASE_URL": "{{base_url}}",
    "ANTHROPIC_AUTH_TOKEN": "{{api_key}}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`},
						{ID: "anthropic-claude-settings-powershell", ClientID: "claude", OS: "powershell", Path: "%userprofile%\\.claude\\settings.json", Hint: "VSCode Claude Code", Language: "json", ContentTemplate: `{
  "env": {
    "ANTHROPIC_BASE_URL": "{{base_url}}",
    "ANTHROPIC_AUTH_TOKEN": "{{api_key}}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`},
					},
				},
			},
			{
				Platform:     "gemini",
				ProviderID:   "v-claw-gemini",
				ProviderName: "Google",
				APIStyle:     "google-native",
				Guide: PlatformGuideMetadata{
					ProfileID:     "gemini",
					Title:         "Gemini CLI guide",
					Description:   "Use this API key with Gemini-compatible clients. The base URL is normalized to the Gemini v1beta endpoint.",
					Note:          "Gemini CLI expects GOOGLE_GEMINI_BASE_URL, GEMINI_API_KEY, and GEMINI_MODEL.",
					DocsURL:       "https://ai.google.dev/gemini-api/docs",
					DefaultClient: "gemini",
					Clients: []PlatformGuideClient{
						{ID: "gemini", Label: "Gemini CLI", OS: []string{"unix", "cmd", "powershell"}},
						{ID: "opencode", Label: "OpenCode"},
					},
					CopyBlocks: []PlatformGuideCopyBlock{
						{ID: "gemini-env-unix", ClientID: "gemini", OS: "unix", Path: "Terminal", Language: "shell", ContentTemplate: `export GOOGLE_GEMINI_BASE_URL="{{gemini_base_url}}"
export GEMINI_API_KEY="{{api_key}}"
export GEMINI_MODEL="{{gemini_model}}"`},
						{ID: "gemini-env-cmd", ClientID: "gemini", OS: "cmd", Path: "Command Prompt", Language: "bat", ContentTemplate: `set GOOGLE_GEMINI_BASE_URL={{gemini_base_url}}
set GEMINI_API_KEY={{api_key}}
set GEMINI_MODEL={{gemini_model}}`},
						{ID: "gemini-env-powershell", ClientID: "gemini", OS: "powershell", Path: "PowerShell", Language: "powershell", ContentTemplate: `$env:GOOGLE_GEMINI_BASE_URL="{{gemini_base_url}}"
$env:GEMINI_API_KEY="{{api_key}}"
$env:GEMINI_MODEL="{{gemini_model}}"`},
					},
				},
			},
		},
	}
}

// DefaultPlatformProfileRegistryJSON returns the default insertable guide registry
// for OpenAI, Anthropic, and Gemini.
func DefaultPlatformProfileRegistryJSON() string {
	blob, _ := json.MarshalIndent(defaultPlatformProfileRegistry(), "", "  ")
	return string(blob)
}

// EffectivePlatformProfileRegistryJSON returns a normalized registry JSON string,
// falling back to the built-in defaults when the stored value is empty or invalid.
func EffectivePlatformProfileRegistryJSON(raw string) string {
	normalized, err := NormalizePlatformProfileRegistryJSON(raw)
	if err != nil {
		return DefaultPlatformProfileRegistryJSON()
	}
	return normalized
}

// NormalizePlatformProfileRegistryJSON validates and pretty-prints a registry payload.
func NormalizePlatformProfileRegistryJSON(raw string) (string, error) {
	registry, err := ParsePlatformProfileRegistry(raw)
	if err != nil {
		return "", err
	}
	blob, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

// ParsePlatformProfileRegistry parses and validates a registry payload.
func ParsePlatformProfileRegistry(raw string) (PlatformProfileRegistry, error) {
	if strings.TrimSpace(raw) == "" {
		registry := defaultPlatformProfileRegistry()
		return registry, validatePlatformProfileRegistry(&registry)
	}
	var registry PlatformProfileRegistry
	if err := json.Unmarshal([]byte(raw), &registry); err != nil {
		return PlatformProfileRegistry{}, infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile registry must be valid JSON")
	}
	if registry.Version == 0 {
		registry.Version = 1
	}
	if err := validatePlatformProfileRegistry(&registry); err != nil {
		return PlatformProfileRegistry{}, err
	}
	return registry, nil
}

func validatePlatformProfileRegistry(registry *PlatformProfileRegistry) error {
	if registry == nil || len(registry.Profiles) == 0 {
		return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile registry must contain at least one profile")
	}
	seenPlatforms := make(map[string]struct{}, len(registry.Profiles))
	for i := range registry.Profiles {
		profile := &registry.Profiles[i]
		profile.Platform = strings.ToLower(strings.TrimSpace(profile.Platform))
		profile.ProviderID = strings.TrimSpace(profile.ProviderID)
		profile.ProviderName = strings.TrimSpace(profile.ProviderName)
		profile.APIStyle = strings.TrimSpace(profile.APIStyle)
		if profile.Platform == "" {
			return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile registry profile.platform is required")
		}
		if _, ok := seenPlatforms[profile.Platform]; ok {
			return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile registry contains duplicate platform: "+profile.Platform)
		}
		seenPlatforms[profile.Platform] = struct{}{}

		guide := &profile.Guide
		guide.ProfileID = strings.TrimSpace(guide.ProfileID)
		if guide.ProfileID == "" {
			guide.ProfileID = profile.Platform
		}
		guide.Title = strings.TrimSpace(guide.Title)
		guide.Description = strings.TrimSpace(guide.Description)
		guide.Note = strings.TrimSpace(guide.Note)
		guide.DocsURL = strings.TrimSpace(guide.DocsURL)
		guide.DefaultClient = strings.TrimSpace(guide.DefaultClient)
		if guide.Title == "" {
			return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile guide.title is required for "+profile.Platform)
		}
		if guide.Description == "" {
			return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile guide.description is required for "+profile.Platform)
		}

		seenClients := make(map[string]struct{}, len(guide.Clients))
		for ci := range guide.Clients {
			client := &guide.Clients[ci]
			client.ID = strings.TrimSpace(client.ID)
			client.Label = strings.TrimSpace(client.Label)
			if client.ID == "" || client.Label == "" {
				return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile guide clients require id and label for "+profile.Platform)
			}
			seenClients[client.ID] = struct{}{}
		}

		seenBlocks := make(map[string]struct{}, len(guide.CopyBlocks))
		for bi := range guide.CopyBlocks {
			block := &guide.CopyBlocks[bi]
			block.ID = strings.TrimSpace(block.ID)
			block.ClientID = strings.TrimSpace(block.ClientID)
			block.OS = strings.ToLower(strings.TrimSpace(block.OS))
			block.Path = strings.TrimSpace(block.Path)
			block.Hint = strings.TrimSpace(block.Hint)
			block.Language = strings.TrimSpace(block.Language)
			if block.ID == "" || block.ClientID == "" || block.Path == "" || strings.TrimSpace(block.ContentTemplate) == "" {
				return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile copy blocks require id, client_id, path, and content_template for "+profile.Platform)
			}
			if _, ok := seenBlocks[block.ID]; ok {
				return infraerrors.BadRequest("INVALID_PLATFORM_PROFILE_REGISTRY", "platform profile copy block ids must be unique for "+profile.Platform)
			}
			seenBlocks[block.ID] = struct{}{}
		}
	}
	return nil
}

// ProfileByPlatform returns a copy of the profile for platform, if present.
func (registry PlatformProfileRegistry) ProfileByPlatform(platform string) (PlatformProfile, bool) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return PlatformProfile{}, false
	}
	for _, profile := range registry.Profiles {
		if strings.EqualFold(profile.Platform, platform) {
			return profile, true
		}
	}
	return PlatformProfile{}, false
}

// ResolvePlatformProfile returns a profile from raw registry JSON, with default
// registry fallback for empty/invalid storage.
func ResolvePlatformProfile(raw, platform string) (PlatformProfile, bool) {
	registry, err := ParsePlatformProfileRegistry(raw)
	if err != nil {
		registry = defaultPlatformProfileRegistry()
	}
	return registry.ProfileByPlatform(platform)
}
