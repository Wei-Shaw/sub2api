import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { getModelsByPlatform, getPresetMappingsByPlatform } from '../useModelWhitelist'

describe('Antigravity default catalog parity', () => {
  it('advertises the same callable IDs as the backend static catalog', () => {
    const source = readFileSync('../backend/internal/pkg/antigravity/claude_types.go', 'utf8')
    const ids = [...source.matchAll(/\{ID: "([^"]+)"/g)].map(match => match[1])
    expect([...getModelsByPlatform('antigravity')].sort()).toEqual(ids.sort())
  })

  it('preserves legacy presets only with live targets or resolvable bare aliases', () => {
    const source = readFileSync('../backend/internal/domain/constants.go', 'utf8')
      .split('var DefaultAntigravityModelMapping =')[1]!.split('\n}')[0]!
    const aliases = new Map([...source.matchAll(/"([^"]+)":\s*"([^"]+)"/g)].map(match => [match[1]!, match[2]!]))
    aliases.set('gemini-3-pro-high', 'gemini-pro-agent')
    aliases.set('gemini-3-pro-preview', 'gemini-pro-agent')
    aliases.set('gemini-3.1-pro', 'gemini-pro-agent')
    aliases.set('gemini-3.1-pro-high', 'gemini-pro-agent')
    aliases.set('gemini-3.1-pro-preview', 'gemini-pro-agent')
    const live = new Set(getModelsByPlatform('antigravity'))
    for (const preset of getPresetMappingsByPlatform('antigravity')) {
      const target = aliases.get(preset.to) ?? preset.to
      expect(live.has(target) || live.has(`${target}-high`), preset.from).toBe(true)
      if (aliases.has(preset.from) && !preset.from.includes('*')) {
        expect(preset.to, preset.from).toBe(aliases.get(preset.from))
      }
    }
  })
})
