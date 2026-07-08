import type { AccountPlatform } from '@/types'

export type AccountApiKeyHintKind = 'baseUrl' | 'apiKey'

export const ACCOUNT_API_KEY_HINT_PLATFORMS = [
  'openai',
  'gemini',
  'grok',
  'kiro',
  'antigravity',
  'deepseek',
  'glm',
  'zai',
  'minimax',
  'opencode'
] as const satisfies readonly AccountPlatform[]

const defaultApiKeyHintKeys: Record<AccountApiKeyHintKind, string> = {
  baseUrl: 'admin.accounts.baseUrlHint',
  apiKey: 'admin.accounts.apiKeyHint'
}

const platformApiKeyHintKeys: Partial<Record<AccountPlatform, Record<AccountApiKeyHintKind, string>>> = {
  openai: {
    baseUrl: 'admin.accounts.openai.baseUrlHint',
    apiKey: 'admin.accounts.openai.apiKeyHint'
  },
  gemini: {
    baseUrl: 'admin.accounts.gemini.baseUrlHint',
    apiKey: 'admin.accounts.gemini.apiKeyHint'
  },
  grok: {
    baseUrl: 'admin.accounts.grok.baseUrlHint',
    apiKey: 'admin.accounts.grok.apiKeyHint'
  },
  kiro: {
    baseUrl: 'admin.accounts.kiro.baseUrlHint',
    apiKey: 'admin.accounts.kiro.apiKeyHint'
  },
  antigravity: {
    baseUrl: 'admin.accounts.antigravity.baseUrlHint',
    apiKey: 'admin.accounts.antigravity.apiKeyHint'
  },
  deepseek: {
    baseUrl: 'admin.accounts.deepseek.baseUrlHint',
    apiKey: 'admin.accounts.deepseek.apiKeyHint'
  },
  glm: {
    baseUrl: 'admin.accounts.glm.baseUrlHint',
    apiKey: 'admin.accounts.glm.apiKeyHint'
  },
  zai: {
    baseUrl: 'admin.accounts.zai.baseUrlHint',
    apiKey: 'admin.accounts.zai.apiKeyHint'
  },
  minimax: {
    baseUrl: 'admin.accounts.minimax.baseUrlHint',
    apiKey: 'admin.accounts.minimax.apiKeyHint'
  },
  opencode: {
    baseUrl: 'admin.accounts.opencode.baseUrlHint',
    apiKey: 'admin.accounts.opencode.apiKeyHint'
  }
}

export function getAccountApiKeyHintKey(
  platform: AccountPlatform | string | null | undefined,
  kind: AccountApiKeyHintKind
): string {
  if (!platform) return defaultApiKeyHintKeys[kind]
  return platformApiKeyHintKeys[platform as AccountPlatform]?.[kind] ?? defaultApiKeyHintKeys[kind]
}
