import type { GroupPlatform } from '@/types'

// Shared by the generated Codex config and the group editor placeholders.
const defaultModels: Record<GroupPlatform, string> = {
  openai: 'gpt-5.6-terra',
  anthropic: 'claude-sonnet-5',
  gemini: 'gemini-3.8-flash',
  antigravity: 'claude-sonnet-5',
  grok: 'grok-4.6',
  kimi: 'kimi-k2.6',
  zhipu: 'glm-5.3-flash',
  deepseek: 'deepseek-flash',
  minimax: 'MiniMax-M3',
  opencode_go: 'glm-5.3',
  composite: 'gpt-5.6-terra'
}

export function getCodexDefaultModel(platform: GroupPlatform): string {
  return defaultModels[platform]
}
