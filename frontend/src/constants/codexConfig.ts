import type { GroupPlatform } from '@/types'

// Shared by generated Codex configs, CC Switch imports, and group editor placeholders.
const defaultModels: Partial<Record<GroupPlatform, { model: string; reviewModel: string }>> = {
  anthropic: { model: 'claude-opus-5-5', reviewModel: 'claude-opus-4-5-20251101' },
  gemini: { model: 'gemini-3.8-flash', reviewModel: 'gemini-3.8-flash' },
  antigravity: { model: 'claude-opus-4-6', reviewModel: 'gemini-3.8-flash' },
  grok: { model: 'grok-4.7', reviewModel: 'grok-4.7' },
  kimi: { model: 'kimi-k3', reviewModel: 'kimi-k3' },
  zhipu: { model: 'glm-5.3', reviewModel: 'glm-5.3-flash' },
  deepseek: { model: 'deepseek-flash', reviewModel: 'deepseek-flash' },
  minimax: { model: 'MiniMax-M3', reviewModel: 'MiniMax-M3' },
  opencode_go: { model: 'glm-5.3', reviewModel: 'glm-5.3-flash' },
  composite: { model: 'gpt-6-sol', reviewModel: 'gpt-6-luna' }
}

export function getCodexDefaultModel(platform: GroupPlatform): string {
  return Object.prototype.hasOwnProperty.call(defaultModels, platform) ? defaultModels[platform]!.model : ''
}

export function getCodexDefaultReviewModel(platform: GroupPlatform): string {
  return Object.prototype.hasOwnProperty.call(defaultModels, platform) ? defaultModels[platform]!.reviewModel : ''
}
