import { describe, expect, it } from 'vitest'
import type { CompositeModelRouteInput } from '@/types'
import { compositeFallbackDefaultModelID, compositeFallbackModelClass, compositeFallbackPlatforms, copyCompositeFallbackTargets, validateCompositeFallback } from '../groupsCompositeFallback'

const route = (overrides: Partial<CompositeModelRouteInput> = {}): CompositeModelRouteInput => ({
  public_model: 'deepseek-v4-pro', match_type: 'exact', target_platform: 'opencode_go',
  upstream_model: 'deepseek-v4-pro', endpoint: 'any',
  fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }],
  ...overrides
})

describe('composite route explicit fallback policy', () => {
  it('keeps the Pro classification distinct from the official default upstream ID', () => {
    const pro = route({ public_model: 'deepseek-pro' })
    expect(compositeFallbackModelClass(pro)).toBe('deepseek-pro')
    expect(compositeFallbackDefaultModelID(pro, 'deepseek')).toBe('deepseek-v4-pro')
    expect(validateCompositeFallback({ ...pro, fallback_targets: [{
      target_platform: 'deepseek', upstream_model: compositeFallbackDefaultModelID(pro, 'deepseek')
    }] })).toBeNull()
  })

  it.each(['deepseek-v4-flash', 'deepseek-v4.1-flash'])('defaults Go %s to official deepseek-flash', model => {
    expect(compositeFallbackDefaultModelID(route({ public_model: 'deepseek-flash', upstream_model: model }), 'deepseek')).toBe('deepseek-flash')
  })

  it.each(['MiniMax-M2.5', 'MiniMax-M2.7', 'MiniMax-M3'])('defaults official %s to the same lowercase Go ID', model => {
    expect(compositeFallbackDefaultModelID(route({ public_model: model, target_platform: 'minimax', upstream_model: model }), 'opencode_go')).toBe(model.toLowerCase())
  })

  it('does not generate a default for an unsupported route', () => {
    expect(compositeFallbackDefaultModelID(route({ match_type: 'prefix' }), 'deepseek')).toBe('')
    expect(compositeFallbackDefaultModelID(route({ upstream_model: 'deepseek-pro' }), 'deepseek')).toBe('')
  })
  it('allows paid DS Go to official and same-model MiniMax official to Go', () => {
    expect(validateCompositeFallback(route())).toBeNull()
    expect(validateCompositeFallback(route({ public_model: 'minimax-m3', target_platform: 'minimax',
      upstream_model: 'MiniMax-M3', fallback_targets: [{ target_platform: 'opencode_go', upstream_model: 'minimax-m3' }] }))).toBeNull()
    expect(validateCompositeFallback(route({ public_model: 'glm-5.3-flash', target_platform: 'zhipu',
      upstream_model: 'glm-5.3-flash', fallback_targets: [
        { target_platform: 'opencode_go', upstream_model: 'glm-5.3-flash' },
        { target_platform: 'openai', upstream_model: 'glm-5.3-flash' }
      ] }))).toBeNull()
    expect(validateCompositeFallback(route({ upstream_model: '' }))).toBeNull()
  })

  it('allows the backend DS Flash aliases without mixing Pro tiers', () => {
    expect(validateCompositeFallback(route({ public_model: 'deepseek-v4-flash', upstream_model: 'deepseek-v4.1-flash',
      fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-flash' }] }))).toBeNull()
    expect(validateCompositeFallback(route({ public_model: 'deepseek-pro',
      fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }] }))).toBeNull()
    expect(validateCompositeFallback(route({ public_model: 'deepseek-pro',
      fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-pro' }] }))).toBe('fallbackPolicyError')
    expect(validateCompositeFallback(route({ public_model: 'deepseek-v4-flash' }))).toBe('fallbackPolicyError')
  })

  it.each([
    { match_type: 'prefix' as const },
    { endpoint: 'images' as const },
    { endpoint: 'count_tokens' as const },
    { upstream_model: 'deepseek-v4-flash-vision-exp' },
    { public_model: 'deepseek-*' },
    { target_platform: 'deepseek' as const },
    { fallback_targets: [{ target_platform: 'opencode_go', upstream_model: 'deepseek-v4-pro' }] },
    { fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-flash' }] },
    { fallback_targets: [
      { target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' },
      { target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }
    ] },
    { fallback_targets: [
      { target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' },
      { target_platform: 'minimax', upstream_model: 'minimax-m3' }
    ] }
  ])('rejects prefix, reverse, cyclic, cross-model or repeated targets: %j', overrides => {
    expect(validateCompositeFallback(route(overrides))).toBe('fallbackPolicyError')
  })

  it.each(['deepseek-v4-pro:free', 'opencode/deepseek-v4-pro-free', 'kimi-k3', 'mimo-v2.5'])('forbids fallback for %s', model => {
    expect(compositeFallbackPlatforms(route({ public_model: model, upstream_model: model }))).toEqual([])
    expect(validateCompositeFallback(route({ upstream_model: model }))).toBe('fallbackPolicyError')
  })

  it('requires a fallback model and permits empty chains on any route', () => {
    expect(validateCompositeFallback(route({ fallback_targets: [{ target_platform: 'deepseek', upstream_model: ' ' }] }))).toBe('fallbackModelRequired')
    expect(validateCompositeFallback(route({ match_type: 'prefix', fallback_targets: [] }))).toBeNull()
    expect(validateCompositeFallback(route({ fallback_targets: undefined }))).toBeNull()
  })

  it('copies ordered targets independently and defaults legacy responses to empty', () => {
    const original = [{ target_platform: 'deepseek', upstream_model: ' deepseek-v4-pro ' }]
    const copied = copyCompositeFallbackTargets(original)
    expect(copied).toEqual([{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }])
    copied[0]!.target_platform = 'minimax'
    expect(original[0]!.target_platform).toBe('deepseek')
    expect(copyCompositeFallbackTargets()).toEqual([])
    expect(copyCompositeFallbackTargets(null)).toEqual([])
  })
})
