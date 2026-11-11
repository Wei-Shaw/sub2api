import { describe, expect, it } from 'vitest'
import { getCodexDefaultModel, getCodexDefaultReviewModel } from '@/constants/codexConfig'

describe('Codex platform defaults', () => {
  it.each(['openai', 'typesafe', 'command_code', 'cline', 'unknown-provider', 'constructor', '__proto__'])(
    'has no model defaults for %s', (platform) => {
      expect(getCodexDefaultModel(platform)).toBe('')
      expect(getCodexDefaultReviewModel(platform)).toBe('')
    }
  )

  it('retains separate main and review defaults for supported platforms', () => {
    expect(getCodexDefaultModel('anthropic')).toBe('claude-opus-5-5')
    expect(getCodexDefaultReviewModel('anthropic')).toBe('claude-opus-4-5-20251101')
  })
})
