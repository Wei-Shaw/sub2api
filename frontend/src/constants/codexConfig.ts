import type { GroupPlatform } from '@/types'

// Shared by the generated Codex config and the group editor placeholders.
const defaultModels: Record<GroupPlatform, string> = {
  openai: 'gpt-5.5',
  anthropic: 'claude-sonnet-4-6',
  gemini: 'gemini-2.5-pro',
  antigravity: 'claude-sonnet-4-6',
  grok: 'grok-4.5',
  kimi: 'kimi-k2.5',
  zhipu: 'glm-4.7',
  deepseek: 'deepseek-v4-pro',
  minimax: 'MiniMax-M3',
  composite: 'gpt-5.5'
}

export function getCodexDefaultModel(platform: GroupPlatform): string {
  return defaultModels[platform]
}
