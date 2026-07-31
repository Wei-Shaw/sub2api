import { describe, expect, it } from 'vitest'
import {
  normalizeOpenAIReasoningEffortPreferences
} from '../openaiReasoningEffortPreferences'

describe('normalizeOpenAIReasoningEffortPreferences', () => {
  it('normalizes aliases, removes invalid values, and preserves canonical order', () => {
    expect(
      normalizeOpenAIReasoningEffortPreferences([' HIGH ', 'x-high', 'low', 'HIGH', 'turbo', 42])
    ).toEqual(['low', 'high', 'xhigh'])
  })

  it('loads backend-compatible comma-separated strings and aliases', () => {
    expect(normalizeOpenAIReasoningEffortPreferences('high, extra-high, LOW, unknown')).toEqual([
      'low',
      'high',
      'xhigh'
    ])
  })

  it('returns an empty selection for missing or malformed config', () => {
    expect(normalizeOpenAIReasoningEffortPreferences(undefined)).toEqual([])
    expect(normalizeOpenAIReasoningEffortPreferences(42)).toEqual([])
  })
})
