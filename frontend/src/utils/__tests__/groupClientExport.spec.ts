import { describe, expect, it } from 'vitest'
import { clientExportFields, filterClientExportModels } from '../groupClientExport'
import { buildCcSwitchImportDeeplink } from '../ccswitchImport'
import type { GroupPlatform } from '@/types'

describe('group client export compatibility', () => {
  it('expands allowlist patterns only against concrete choices', () => {
    expect(filterClientExportModels(['gpt-5.6-sol', 'gpt-5.6-luna', 'gpt-*'], { enabled: true, models: ['gpt-*-sol'] })).toEqual(['gpt-5.6-sol'])
  })

  it.each(['openai', 'grok', 'gemini', 'antigravity', 'anthropic', 'deepseek', 'minimax', 'composite'] as GroupPlatform[])('preserves native client and endpoint for %s', platform => {
    const input = { baseUrl: 'https://example.com', providerName: 'Test', apiKey: 'sk-test', usageScript: '', clientType: 'claude' as const, platform }
    const original = new URLSearchParams(buildCcSwitchImportDeeplink(input).split('?')[1])
    const changed = new URLSearchParams(buildCcSwitchImportDeeplink({ ...input, clientExportConfig: { codex_model: 'group-codex', grok_model: 'group-grok', gemini_model: 'group-gemini' } }).split('?')[1])
    expect(changed.get('app')).toBe(original.get('app'))
    expect(changed.get('endpoint')).toBe(original.get('endpoint'))
    expect(changed.get('model')).toBe(platform === 'openai' ? 'group-codex' : platform === 'grok' ? 'group-grok' : platform === 'gemini' ? 'group-gemini' : original.get('model'))
  })

  it('does not expose generic editable fields for the dedicated TypeSafe template', () => {
    expect(clientExportFields('typesafe')).toEqual([])
  })
})
