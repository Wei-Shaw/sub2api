import { describe, expect, it } from 'vitest'
import {
  COMPOSITE_TARGET_PLATFORM_OPTIONS,
  CONCRETE_PLATFORM_OPTIONS,
  GROUP_PLATFORM_OPTIONS
} from '@/constants/platforms'

const concretePlatforms = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'kimi',
  'zhipu',
  'deepseek',
  'minimax',
  'mimo'
]

describe('platform option catalogs', () => {
  it('exposes every concrete account platform', () => {
    expect(CONCRETE_PLATFORM_OPTIONS.map((option) => option.value)).toEqual(concretePlatforms)
  })

  it('adds composite for group-backed filters', () => {
    expect(GROUP_PLATFORM_OPTIONS.map((option) => option.value)).toEqual([
      ...concretePlatforms,
      'composite'
    ])
  })

  it('keeps new direct providers out of Composite targets until backend routing supports them', () => {
    expect(COMPOSITE_TARGET_PLATFORM_OPTIONS.map(option => option.value)).not.toEqual(
      expect.arrayContaining(['minimax', 'mimo'])
    )
  })
})
