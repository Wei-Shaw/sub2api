import { describe, expect, it } from 'vitest'
import type { AccountPlatform } from '../../types'
import { getAccountApiKeyHintKey } from '../accountPlatformHints'

describe('accountPlatformHints', () => {
  it('returns platform-specific API-key helper keys for every OpenAI-compatible account platform', () => {
    const cases: Array<{ platform: AccountPlatform; prefix: string }> = [
      { platform: 'openai', prefix: 'openai' },
      { platform: 'gemini', prefix: 'gemini' },
      { platform: 'grok', prefix: 'grok' },
      { platform: 'kiro', prefix: 'kiro' },
      { platform: 'antigravity', prefix: 'antigravity' },
      { platform: 'deepseek', prefix: 'deepseek' },
      { platform: 'glm', prefix: 'glm' },
      { platform: 'zai', prefix: 'zai' },
      { platform: 'minimax', prefix: 'minimax' },
      { platform: 'opencode', prefix: 'opencode' }
    ]

    for (const { platform, prefix } of cases) {
      expect(getAccountApiKeyHintKey(platform, 'baseUrl')).toBe(`admin.accounts.${prefix}.baseUrlHint`)
      expect(getAccountApiKeyHintKey(platform, 'apiKey')).toBe(`admin.accounts.${prefix}.apiKeyHint`)
    }
  })

  it('keeps Anthropic and unknown platforms on the generic Anthropic-compatible copy', () => {
    expect(getAccountApiKeyHintKey('anthropic', 'baseUrl')).toBe('admin.accounts.baseUrlHint')
    expect(getAccountApiKeyHintKey('anthropic', 'apiKey')).toBe('admin.accounts.apiKeyHint')
    expect(getAccountApiKeyHintKey('unknown-platform', 'baseUrl')).toBe('admin.accounts.baseUrlHint')
    expect(getAccountApiKeyHintKey(null, 'apiKey')).toBe('admin.accounts.apiKeyHint')
  })
})
