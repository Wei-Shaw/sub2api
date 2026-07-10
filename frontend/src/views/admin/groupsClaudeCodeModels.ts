import type { ClaudeCodeDefaultModels } from "@/types";

// Form state mirrors the flat fields stored on the group form (claude_code_*
// prefixed to stay unique inside the large createForm/editForm objects).
// Empty = "do not emit the corresponding ANTHROPIC_DEFAULT_*_MODEL", so this
// is generic across upstreams.
export interface ClaudeCodeModelsFormState {
  claude_code_haiku_model: string;
  claude_code_sonnet_model: string;
  claude_code_opus_model: string;
}

export function createDefaultClaudeCodeModelsFormState(): ClaudeCodeModelsFormState {
  return {
    claude_code_haiku_model: "",
    claude_code_sonnet_model: "",
    claude_code_opus_model: "",
  };
}

export function claudeCodeModelsConfigToFormState(
  config?: ClaudeCodeDefaultModels | null,
): ClaudeCodeModelsFormState {
  return {
    claude_code_haiku_model: config?.haiku?.trim() || "",
    claude_code_sonnet_model: config?.sonnet?.trim() || "",
    claude_code_opus_model: config?.opus?.trim() || "",
  };
}

export function claudeCodeModelsFormStateToConfig(
  state: ClaudeCodeModelsFormState,
): ClaudeCodeDefaultModels {
  const config: ClaudeCodeDefaultModels = {};
  const haiku = state.claude_code_haiku_model.trim();
  const sonnet = state.claude_code_sonnet_model.trim();
  const opus = state.claude_code_opus_model.trim();
  if (haiku) config.haiku = haiku;
  if (sonnet) config.sonnet = sonnet;
  if (opus) config.opus = opus;
  return config;
}

export function resetClaudeCodeModelsFormState(
  target: ClaudeCodeModelsFormState,
): void {
  const defaults = createDefaultClaudeCodeModelsFormState();
  target.claude_code_haiku_model = defaults.claude_code_haiku_model;
  target.claude_code_sonnet_model = defaults.claude_code_sonnet_model;
  target.claude_code_opus_model = defaults.claude_code_opus_model;
}
