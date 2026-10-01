package cursor

// defaultRunSlugTable is a snapshot of AvailableModels field 36 (LegacySlugs).
// AgentService/Run accepts these parameterized ids, not always the picker Name.
// TestE2ECursorRunSlugSnapshotMatchesLive refreshes this against a live catalog.
func defaultRunSlugTable() map[string][]string {
	return map[string][]string{
		"grok-4.6": effortFast("cursor-grok-4.6", "low", "medium", "high", "xhigh"),
		"composer-2.5": {
			"composer-2.5-fast",
		},
		"claude-opus-5": concat(
			effortFast("claude-opus-5", "low", "medium", "high"),
			effortFast("claude-opus-5-thinking", "low", "medium", "high", "xhigh", "max"),
		),
		"claude-opus-4-8": concat(
			effortFast("claude-opus-4-8", "low", "medium", "high", "xhigh", "max"),
			effortFast("claude-opus-4-8-thinking", "low", "medium", "high", "xhigh", "max"),
		),
		"gpt-5.6-sol": effortFast("gpt-5.6-sol", "none", "low", "medium", "high", "xhigh", "max"),
		"gpt-5.5": {
			"gpt-5.5-none", "gpt-5.5-none-fast",
			"gpt-5.5-low", "gpt-5.5-low-fast",
			"gpt-5.5-medium", "gpt-5.5-medium-fast",
			"gpt-5.5-high", "gpt-5.5-high-fast",
			"gpt-5.5-extra-high", "gpt-5.5-extra-high-fast",
		},
		"claude-fable-5": concat(
			effortOnly("claude-fable-5", "low", "medium", "high", "xhigh", "max"),
			effortOnly("claude-fable-5-thinking", "low", "medium", "high", "xhigh", "max"),
		),
		"grok-4.5": concat(
			effortFast("cursor-grok-4.5", "low", "medium", "high"),
			[]string{
				"grok-4.5-medium", "grok-4.5-fast-medium",
				"grok-4.5-high", "grok-4.5-fast-high",
				"grok-4.5-xhigh", "grok-4.5-fast-xhigh",
			},
		),
		"gemini-3.7-flash": effortOnly("gemini-3.7-flash", "low", "medium", "high"),
		"gpt-5.6-terra":    effortFast("gpt-5.6-terra", "none", "low", "medium", "high", "xhigh", "max"),
		"claude-sonnet-5": concat(
			effortOnly("claude-sonnet-5", "low", "medium", "high", "xhigh", "max"),
			effortOnly("claude-sonnet-5-thinking", "low", "medium", "high", "xhigh", "max"),
		),
		"claude-sonnet-4-6": concat(
			effortOnly("claude-4.6-sonnet", "low", "medium", "high", "max"),
			thinkingSuffix("claude-4.6-sonnet", "low", "medium", "high", "max"),
		),
		"gpt-5.3-codex": {
			"gpt-5.3-codex-low", "gpt-5.3-codex-low-fast",
			"gpt-5.3-codex-fast",
			"gpt-5.3-codex-high", "gpt-5.3-codex-high-fast",
			"gpt-5.3-codex-xhigh", "gpt-5.3-codex-xhigh-fast",
		},
		"claude-opus-4-7": concat(
			effortFast("claude-opus-4-7", "low", "medium", "high", "xhigh", "max"),
			effortFast("claude-opus-4-7-thinking", "low", "medium", "high", "xhigh", "max"),
		),
		"gpt-5.4": effortFast("gpt-5.4", "none", "low", "medium", "high", "xhigh"),
		"claude-opus-4-6": concat(
			effortOnly("claude-4.6-opus", "low", "medium", "high", "max"),
			thinkingSuffix("claude-4.6-opus", "low", "medium", "high", "max"),
		),
		"claude-opus-4-5": {
			"claude-4.5-opus-high",
			"claude-4.5-opus-high-thinking",
		},
		"gpt-5.2": {
			"gpt-5.2-low", "gpt-5.2-low-fast",
			"gpt-5.2-fast",
			"gpt-5.2-high", "gpt-5.2-high-fast",
			"gpt-5.2-xhigh", "gpt-5.2-xhigh-fast",
		},
		"gpt-5.6-luna":     effortFast("gpt-5.6-luna", "none", "low", "medium", "high", "xhigh", "max"),
		"gemini-3.6-flash": effortOnly("gemini-3.6-flash", "minimal", "low", "medium", "high"),
		"gpt-5.4-mini":     effortOnly("gpt-5.4-mini", "none", "low", "medium", "high", "xhigh"),
		"gpt-5.4-nano":     effortOnly("gpt-5.4-nano", "none", "low", "medium", "high", "xhigh"),
		"claude-haiku-4-5": {
			"claude-4.5-haiku",
			"claude-4.5-haiku-thinking",
		},
		"claude-sonnet-4-5": {
			"claude-4.5-sonnet",
			"claude-4.5-sonnet-thinking",
		},
		"gpt-5.1": {
			"gpt-5.1-low",
			"gpt-5.1-high",
		},
		"claude-sonnet-4": {
			"claude-4-sonnet",
			"claude-4-sonnet-thinking",
		},
		"kimi-k3": {
			"kimi-k3-low",
			"kimi-k3-high",
			"kimi-k3-max",
		},
		"glm-5.2": {
			"glm-5.2-high",
			"glm-5.2-max",
		},
	}
}

func effortFast(stem string, efforts ...string) []string {
	out := make([]string, 0, len(efforts)*2)
	for _, effort := range efforts {
		out = append(out, stem+"-"+effort, stem+"-"+effort+"-fast")
	}
	return out
}

func effortOnly(stem string, efforts ...string) []string {
	out := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		out = append(out, stem+"-"+effort)
	}
	return out
}

func thinkingSuffix(stem string, efforts ...string) []string {
	out := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		out = append(out, stem+"-"+effort+"-thinking")
	}
	return out
}

func concat(parts ...[]string) []string {
	n := 0
	for _, part := range parts {
		n += len(part)
	}
	out := make([]string, 0, n)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}
