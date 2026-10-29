import { describe, expect, it } from 'vitest'
import { findLatestFlagshipModel, selectCodexCatalogModel } from '@/utils/flagshipModel'

describe('findLatestFlagshipModel', () => {
  it('selects gpt-6-astra over gpt-5.5', () => {
    expect(findLatestFlagshipModel(['gpt-5.5', 'gpt-6-astra', 'gpt-4o'])).toBe('gpt-6-astra')
  })

  it('selects the highest version among flagships (gpt-6.1-nova over gpt-6-astra)', () => {
    expect(findLatestFlagshipModel(['gpt-5.6-sol', 'gpt-6.1-nova', 'gpt-6-astra'])).toBe(
      'gpt-6.1-nova'
    )
  })

  it('prefers Anthropic sonnet-5+ among newer Claude flagships', () => {
    expect(
      findLatestFlagshipModel(['claude-sonnet-4-6', 'claude-sonnet-5-5', 'claude-opus-5'])
    ).toBe('claude-sonnet-5-5')
  })

  it('returns null when catalog has no flagship (legacy gpt-4 only)', () => {
    expect(findLatestFlagshipModel(['gpt-4o', 'gpt-4-turbo'])).toBeNull()
  })

  it('returns null for an empty catalog', () => {
    expect(findLatestFlagshipModel([])).toBeNull()
  })

  it('recognizes gemini/grok only when weight >= 500 (major >= 5)', () => {
    // Same threshold as gpt/claude: major*100 >= 500. gemini-3 (=300) is not flagship yet.
    expect(findLatestFlagshipModel(['gemini-2.5-pro', 'gemini-3-pro'])).toBeNull()
    expect(findLatestFlagshipModel(['gemini-2.5-pro', 'gemini-5-pro'])).toBe('gemini-5-pro')
    expect(findLatestFlagshipModel(['grok-4.5', 'grok-5-apex'])).toBe('grok-5-apex')
  })
})

describe('selectCodexCatalogModel', () => {
  it('prefers latest flagship over the preferred model', () => {
    expect(selectCodexCatalogModel(['gpt-5.5', 'gpt-6-astra', 'gpt-4o'], 'gpt-5.5')).toBe(
      'gpt-6-astra'
    )
  })

  it('selects highest-version flagship when several exist', () => {
    expect(
      selectCodexCatalogModel(['gpt-5.6-sol', 'gpt-6.1-nova', 'gpt-6-astra'], 'gpt-5.5')
    ).toBe('gpt-6.1-nova')
  })

  it('falls back to preferred when it is in the catalog and no flagship exists', () => {
    expect(selectCodexCatalogModel(['gpt-4o', 'gpt-4-turbo'], 'gpt-4o')).toBe('gpt-4o')
  })

  it('falls back to the first catalog entry when preferred is missing and no flagship', () => {
    expect(selectCodexCatalogModel(['gpt-4o', 'gpt-4-turbo'], 'gpt-5.5')).toBe('gpt-4o')
  })

  it('returns preferred for an empty catalog', () => {
    expect(selectCodexCatalogModel([], 'gpt-5.5')).toBe('gpt-5.5')
  })

  it('selects Anthropic flagship from mixed catalog', () => {
    expect(
      selectCodexCatalogModel(
        ['claude-sonnet-4-6', 'claude-sonnet-5-5', 'claude-opus-5'],
        'claude-sonnet-4-6'
      )
    ).toBe('claude-sonnet-5-5')
  })
})
