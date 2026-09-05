package claude

import "testing"

func TestResolveProfileReturnsVersionedIdentities(t *testing.T) {
	tests := []struct {
		id         string
		entrypoint string
		promptPart string
	}{
		{ProfileClaudeCodeCLI, "cli", "official CLI"},
		{ProfileClaudeAgentSDK, "sdk", "Agent SDK"},
		{ProfileClaudeCodeIDE, "claude-vscode", "official CLI"},
	}
	for _, tt := range tests {
		profile := ResolveProfile(tt.id)
		if profile.ID != tt.id || profile.Version == "" || profile.Entrypoint != tt.entrypoint {
			t.Fatalf("ResolveProfile(%q) = %#v", tt.id, profile)
		}
		if profile.UserAgent == "" || profile.SystemPrompt == "" || profile.CacheControlTTL == "" {
			t.Fatalf("profile %q is incomplete: %#v", tt.id, profile)
		}
		if !contains(profile.SystemPrompt, tt.promptPart) {
			t.Fatalf("profile %q prompt %q does not contain %q", tt.id, profile.SystemPrompt, tt.promptPart)
		}
	}
}

func TestResolveProfileIsDefensive(t *testing.T) {
	first := ResolveProfile(ProfileClaudeCodeCLI)
	first.Betas[0] = "mutated"
	first.Headers["X-Test"] = "mutated"
	second := ResolveProfile(ProfileClaudeCodeCLI)
	if second.Betas[0] == "mutated" || second.Headers["X-Test"] != "" {
		t.Fatal("ResolveProfile returned shared mutable state")
	}
}

func TestNormalizeMimicMode(t *testing.T) {
	if NormalizeMimicMode(" STRICT ") != MimicModeStrict {
		t.Fatal("strict mode was not normalized")
	}
	if NormalizeMimicMode("unknown") != MimicModeCompatibility {
		t.Fatal("unknown mode must fail closed to compatibility")
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
