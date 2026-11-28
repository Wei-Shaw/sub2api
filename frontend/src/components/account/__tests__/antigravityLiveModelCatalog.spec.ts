import { describe, expect, it } from 'vitest'
import {
  applyAntigravityModelMappingMode,
  deriveAntigravityLiveModelNames,
  isAntigravityLiveModelMapping,
  readAntigravityModelCatalog
} from '../antigravityLiveModelCatalog'

// fetchAvailableModels catalog of the Ultra accounts on 2026-11-23.
const ULTRA_CATALOG = [
  'claude-opus-5-5-high', 'claude-opus-5-5-low', 'claude-opus-5-5-medium',
  'claude-sonnet-5-5-high', 'claude-sonnet-5-5-low', 'claude-sonnet-5-5-medium',
  'gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.5-flash-thinking', 'gemini-2.5-pro',
  'gemini-3-flash', 'gemini-3.1-flash-image', 'gemini-3.1-flash-lite', 'gemini-3.5-flash-lite',
  'gemini-3.8-flash-high', 'gemini-3.8-flash-low', 'gemini-3.8-flash-medium', 'gemini-3.8-flash-tiered',
  'gemini-4-argon-high', 'gemini-4-argon-low', 'gemini-4-argon-medium',
  'gemini-4-carbon-high', 'gemini-4-carbon-low', 'gemini-4-carbon-medium',
  'gemini-4-hydrogen-high', 'gemini-4-hydrogen-low', 'gemini-4-hydrogen-medium', 'gemini-4-hydrogen-xhigh',
  'gemini-4-hydrogen-sonar', 'gemini-4-hydrogen-tiered-high', 'gemini-4-hydrogen-tiered-xhigh',
  'gemini-4-neon-high', 'gemini-4-neon-low', 'gemini-4-neon-medium', 'gemini-4-neon-xhigh',
  'gpt-oss-120b-medium', 'tab_flash_lite_preview', 'tab_jump_flash_lite_preview'
]

describe('antigravityLiveModelCatalog', () => {
  it('derives the same names as the backend mapping for the Ultra catalog', () => {
    expect(deriveAntigravityLiveModelNames([...ULTRA_CATALOG, 'chat_20706-disabled'])).toEqual([
      'claude-opus-5-5',
      'claude-sonnet-5-5',
      'gemini-2.5-flash',
      'gemini-2.5-flash-lite',
      'gemini-2.5-flash-thinking',
      'gemini-2.5-pro',
      'gemini-3-flash',
      'gemini-3.1-flash-image',
      'gemini-3.1-flash-lite',
      'gemini-3.5-flash-lite',
      'gemini-3.8-flash',
      'gemini-3.8-flash-tiered',
      'gemini-4-argon',
      'gemini-4-carbon',
      'gemini-4-hydrogen',
      'gemini-4-hydrogen-sonar',
      'gemini-4-hydrogen-tiered',
      'gemini-4-neon',
      'gpt-oss-120b-medium',
      'tab_flash_lite_preview',
      'tab_jump_flash_lite_preview'
    ])
  })

  it('reads the saved catalog from extra and ignores unusable values', () => {
    expect(
      readAntigravityModelCatalog({
        antigravity_model_catalog: { models: [' gemini-3-flash ', 'x-disabled', 3], synced_at: '2026-11-23T10:00:00Z' }
      })
    ).toEqual({ models: ['gemini-3-flash'], syncedAt: '2026-11-23T10:00:00Z' })
    expect(readAntigravityModelCatalog({ antigravity_model_catalog: { models: [] } })).toBeNull()
    expect(readAntigravityModelCatalog({})).toBeNull()
    expect(readAntigravityModelCatalog(undefined)).toBeNull()
  })

  it('sets and clears the live mode flag', () => {
    const credentials: Record<string, unknown> = { access_token: 'x' }
    applyAntigravityModelMappingMode(credentials, true)
    expect(credentials.model_mapping_mode).toBe('live')
    expect(isAntigravityLiveModelMapping(credentials)).toBe(true)
    applyAntigravityModelMappingMode(credentials, false)
    expect(credentials).not.toHaveProperty('model_mapping_mode')
    expect(isAntigravityLiveModelMapping(credentials)).toBe(false)
    expect(isAntigravityLiveModelMapping({ model_mapping_mode: 'static' })).toBe(false)
  })
})
