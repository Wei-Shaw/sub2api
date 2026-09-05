package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
)

func TestClaudeMimicCompatibilityGoldenShape(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","system":"Project instructions","messages":[{"role":"user","content":"hello"}]}`)
	profile := claude.ResolveProfile(claude.ProfileClaudeCodeCLI)
	out := rewriteSystemForNonClaudeCodeWithPromptBlocksMode(body, "Project instructions", "", "", profile, claude.MimicModeCompatibility)

	system := gjson.GetBytes(out, "system")
	if !system.IsArray() || len(system.Array()) != 3 {
		t.Fatalf("compatibility system shape = %s", system.Raw)
	}
	if system.Get("0.text").String() == "" || system.Get("1.text").String() != profile.SystemPrompt {
		t.Fatalf("compatibility profile blocks = %s", system.Raw)
	}
	if gjson.GetBytes(out, "messages.0.role").String() != "user" ||
		gjson.GetBytes(out, "messages.1.role").String() != "assistant" {
		t.Fatalf("compatibility did not preserve the system-to-messages bridge: %s", out)
	}
	if gjson.GetBytes(out, "messages.1.content.0.text").String() != "Understood. I will follow these instructions." {
		t.Fatalf("compatibility acknowledgement changed: %s", out)
	}
}

func TestClaudeMimicStrictGoldenShape(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","system":"Project instructions","messages":[{"role":"user","content":"hello"}]}`)
	profile := claude.ResolveProfile(claude.ProfileClaudeAgentSDK)
	out := rewriteSystemForNonClaudeCodeWithPromptBlocksMode(body, "Project instructions", "", "", profile, claude.MimicModeStrict)

	system := gjson.GetBytes(out, "system")
	if !system.IsArray() || len(system.Array()) != 4 {
		t.Fatalf("strict system shape = %s", system.Raw)
	}
	if system.Get("0.text").String() == "" || system.Get("1.text").String() != profile.SystemPrompt {
		t.Fatalf("strict profile blocks = %s", system.Raw)
	}
	if system.Get("1.text").String() == claude.ResolveProfile(claude.ProfileClaudeCodeCLI).SystemPrompt {
		t.Fatal("strict profile ignored the selected Agent SDK identity")
	}
	if system.Get("2.cache_control.ttl").String() != profile.CacheControlTTL {
		t.Fatalf("strict cache TTL = %s, want %s", system.Get("2.cache_control.ttl").String(), profile.CacheControlTTL)
	}
	if system.Get("3.text").String() != "Project instructions" {
		t.Fatalf("strict original system block = %s", system.Get("3").Raw)
	}
	if len(gjson.GetBytes(out, "messages").Array()) != 1 {
		t.Fatalf("strict mode synthesized a conversation turn: %s", out)
	}
}

func TestClaudeMimicBillingEntrypointGolden(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	profile := claude.ResolveProfile(claude.ProfileClaudeCodeIDE)
	text, err := buildBillingAttributionTextWithEntrypoint(body, profile.Version, profile.Entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if want := "cc_entrypoint=" + profile.Entrypoint + ";"; !containsString(text, want) {
		t.Fatalf("billing attribution = %q, want %q", text, want)
	}
}

func TestClaudeMimicStrictGoldenFixture(t *testing.T) {
	fixturePath := filepath.Join("testdata", "claude_mimic", "strict_agent_sdk.json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Profile  string `json:"profile"`
		Mode     string `json:"mode"`
		Input    []byte `json:"-"`
		Expected struct {
			SystemBlockCount  int    `json:"system_block_count"`
			BillingPrefix     string `json:"billing_prefix"`
			BillingEntrypoint string `json:"billing_entrypoint"`
			IdentityPrompt    string `json:"identity_prompt"`
			ExpansionContains string `json:"expansion_contains"`
			CacheTTL          string `json:"cache_ttl"`
			OriginalSystem    string `json:"original_system"`
			MessageCount      int    `json:"message_count"`
		} `json:"expected"`
		InputObject map[string]any `json:"input"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(fixture.InputObject)
	if err != nil {
		t.Fatal(err)
	}
	out := rewriteSystemForNonClaudeCodeWithPromptBlocksMode(
		input,
		fixture.InputObject["system"],
		"",
		"",
		claude.ResolveProfile(fixture.Profile),
		claude.NormalizeMimicMode(fixture.Mode),
	)
	system := gjson.GetBytes(out, "system")
	if len(system.Array()) != fixture.Expected.SystemBlockCount {
		t.Fatalf("golden system block count=%d, want %d", len(system.Array()), fixture.Expected.SystemBlockCount)
	}
	if !containsString(system.Get("0.text").String(), fixture.Expected.BillingPrefix) ||
		!containsString(system.Get("0.text").String(), fixture.Expected.BillingEntrypoint) {
		t.Fatalf("golden billing block=%q", system.Get("0.text").String())
	}
	if system.Get("1.text").String() != fixture.Expected.IdentityPrompt {
		t.Fatalf("golden identity=%q", system.Get("1.text").String())
	}
	if !containsString(system.Get("2.text").String(), fixture.Expected.ExpansionContains) ||
		system.Get("2.cache_control.ttl").String() != fixture.Expected.CacheTTL {
		t.Fatalf("golden expansion/cache block=%s", system.Get("2").Raw)
	}
	if system.Get("3.text").String() != fixture.Expected.OriginalSystem {
		t.Fatalf("golden original system=%q", system.Get("3.text").String())
	}
	if len(gjson.GetBytes(out, "messages").Array()) != fixture.Expected.MessageCount {
		t.Fatalf("golden message count=%d, want %d", len(gjson.GetBytes(out, "messages").Array()), fixture.Expected.MessageCount)
	}
}

func containsString(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
