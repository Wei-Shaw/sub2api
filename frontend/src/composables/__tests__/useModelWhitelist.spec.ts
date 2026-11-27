import { describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import { buildModelMappingObject, getModelsByPlatform, getPresetMappingsByPlatform, removeModelMappingEntry, splitModelMappingObject } from '../useModelWhitelist'
import { BUILTIN_PLATFORM_CATALOG, resetPlatformCatalog, setPlatformCatalog } from '@/constants/platformCatalog'

describe('useModelWhitelist', () => {
  it('keeps persisted provenance out of enumerable fields and JSON', () => {
    const { modelMappings } = splitModelMappingObject({ A: 'C' })
    expect(Object.keys(modelMappings[0])).toEqual(['from', 'to'])
    expect(JSON.stringify(modelMappings)).toBe('[{"from":"A","to":"C"}]')
  })

  it.each(['', '   ', 'new-*'])('retains the original source after editing a loaded source to %j', (from) => {
    const state = reactive(splitModelMappingObject({ A: 'C' }))
    state.modelMappings[0].from = from
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(state.allowedModels).toEqual(['A'])
  })

  it.each(['', '   ', 'target-*', '*target'])('does not whitelist a cancelled invalid draft with target %j', (to) => {
    const state = splitModelMappingObject({ B: 'B' })
    state.modelMappings.push({ from: 'A', to })
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(buildModelMappingObject('combined', state.allowedModels, state.modelMappings)).toEqual({ B: 'B' })
  })

  it.each(['', '   ', 'target-*'])('retains a loaded source after changing its target to %j', (to) => {
    const state = reactive(splitModelMappingObject({ A: 'C', B: 'B' }))
    state.modelMappings[0].to = to
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(buildModelMappingObject('combined', state.allowedModels, state.modelMappings)).toEqual({ A: 'A', B: 'B' })
    expect(buildModelMappingObject('mapping', state.allowedModels, state.modelMappings)).toBeNull()
  })

  it('does not transfer loaded provenance to a replacement draft or another account', () => {
    const state = reactive(splitModelMappingObject({ A: 'C', B: 'B' }))
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    state.allowedModels.splice(state.allowedModels.indexOf('A'), 1)
    state.modelMappings.push({ from: 'A', to: '' })
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(state.allowedModels).toEqual(['B'])
    const other = splitModelMappingObject({ B: 'B' })
    other.modelMappings.push({ from: 'A', to: '' })
    removeModelMappingEntry(other.modelMappings, 0, other.allowedModels)
    expect(other.allowedModels).toEqual(['B'])
  })

  it('preserves the remaining same-source rewrite when deleting an edited loaded row', () => {
    const state = splitModelMappingObject({ A: 'C' })
    state.modelMappings[0].to = ''
    state.modelMappings.push({ from: 'A', to: 'D' })
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(buildModelMappingObject('combined', state.allowedModels, state.modelMappings)).toEqual({ A: 'D' })
    expect(JSON.stringify(state.modelMappings)).toBe('[{"from":"A","to":"D"}]')
  })

  it('restores only the persisted source when both inputs of a loaded row were edited to an invalid draft', () => {
    const state = splitModelMappingObject({ A: 'C' })
    state.modelMappings[0].from = 'new-model'
    state.modelMappings[0].to = ''
    removeModelMappingEntry(state.modelMappings, 0, state.allowedModels)
    expect(state.allowedModels).toEqual(['A'])
  })

  it('平台清单中没有内置模型列表的多协议供应商不预填白名单', () => {
    setPlatformCatalog({
      ...BUILTIN_PLATFORM_CATALOG,
      platforms: [
        ...BUILTIN_PLATFORM_CATALOG.platforms,
        {
          id: 'acme_router',
          display_name: 'Acme Router',
          gateway: 'openai',
          cn_provider: false,
          multi_protocol: {
            default_mode: 'standard',
            routing: 'by_model',
            modes: [{ mode: 'standard', base_urls: { chat_completions: 'https://api.acme-router.example/v1' } }]
          }
        }
      ]
    })
    try {
      expect(getModelsByPlatform('acme_router')).toEqual([])
      expect(getModelsByPlatform('kimi').length).toBeGreaterThan(0)
      // 非多协议供应商的未知平台维持原有回退。
      expect(getModelsByPlatform('bedrock')).toEqual(getModelsByPlatform('anthropic'))
    } finally {
      resetPlatformCatalog()
    }
  })

  it('openai 模型列表包含 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('codex-auto-review')
    expect(models).toContain('gpt-5.6')
    expect(models).toContain('gpt-6')
    expect(models).toContain('gpt-6-astra')
    expect(models).toContain('gpt-6.2-sol')
    expect(models).toContain('gpt-6.2-astra')
    expect(models).toContain('gpt-6.1-sol')
    expect(models).toContain('gpt-6.1-terra')
    expect(models).toContain('gpt-6-sol')
    expect(models).toContain('gpt-6-luna')
    expect(models).toContain('gpt-image-2.6-flare')
    expect(models).toContain('gpt-image-2.6-sunburst')
  })

  it('openai 预设映射包含 GPT-6 别名和 Astra', () => {
    expect(getPresetMappingsByPlatform('openai')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'GPT-6.2 Sol', from: 'gpt-6.2-sol', to: 'gpt-6.2-sol' }),
      expect.objectContaining({ label: 'GPT-6.2 Astra', from: 'gpt-6.2-astra', to: 'gpt-6.2-astra' }),
      expect.objectContaining({ label: 'GPT-6.1 Sol', from: 'gpt-6.1-sol', to: 'gpt-6.1-sol' }),
      expect.objectContaining({ label: 'GPT-6.1 Terra', from: 'gpt-6.1-terra', to: 'gpt-6.1-terra' }),
      expect.objectContaining({ label: 'GPT-6', from: 'gpt-6', to: 'gpt-6' }),
      expect.objectContaining({ label: 'GPT-6 Astra', from: 'gpt-6-astra', to: 'gpt-6-astra' })
    ]))
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5-1')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5-1')
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-5-5')
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-opus-5-5')
    expect(getModelsByPlatform('claude')).toContain('claude-sonnet-5-5')
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-sonnet-5-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it.each([
    ['claude-fable-5-2', 'Fable 5.2'],
    ['claude-mythos-5-2', 'Mythos 5.2']
  ])('preserves %s in Anthropic whitelists and presets without inventing provider mappings', (model, label) => {
    for (const platform of ['claude', 'anthropic']) {
      const models = getModelsByPlatform(platform)
      expect(models).toContain(model)
      expect(buildModelMappingObject('whitelist', models, [])?.[model]).toBe(model)
      expect(getPresetMappingsByPlatform(platform)).toEqual(expect.arrayContaining([
        expect.objectContaining({ label, from: model, to: model })
      ]))
    }
    expect(getPresetMappingsByPlatform('bedrock')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label, from: model, to: `anthropic.${model}` })
    ]))
    expect(getModelsByPlatform('antigravity')).not.toContain(model)
    expect(getPresetMappingsByPlatform('antigravity').some(mapping => mapping.from === model)).toBe(false)
  })

  it('Claude Sonnet 5.5 预设使用各平台的官方模型 ID', () => {
    expect(getPresetMappingsByPlatform('claude')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'Sonnet 5.5', from: 'claude-sonnet-5-5', to: 'claude-sonnet-5-5' })
    ]))
    expect(getPresetMappingsByPlatform('bedrock')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'Sonnet 5.5', from: 'claude-sonnet-5-5', to: 'global.anthropic.claude-sonnet-5-5' })
    ]))
  })

  it('Claude 默认白名单保存后允许 Haiku 5.5，预设使用同一模型 ID', () => {
    const models: string[] = getModelsByPlatform('anthropic')
    const mapping: Record<string, string> | null = buildModelMappingObject('whitelist', models, [])

    expect(mapping?.['claude-haiku-5-5']).toBe('claude-haiku-5-5')
    expect(getPresetMappingsByPlatform('anthropic')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'Haiku 5.5', from: 'claude-haiku-5-5', to: 'claude-haiku-5-5' })
    ]))
  })

  it('xAI 模型列表包含 Grok 4.5 官方模型和别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-4.6')
    expect(models).toContain('grok-4.6-latest')
    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-4.5-latest')
    expect(models).toContain('grok-build-latest')
    expect(models).toContain('grok-imagine-image-2.0')
    expect(models).toContain('grok-imagine-video-1.5')
  })

  it('combined 模式支持 Grok 4.5 官方别名映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['grok-4.5'],
      [
        { from: 'grok-latest', to: 'grok-4.5' },
        { from: 'grok-4.5-latest', to: 'grok-4.5' },
        { from: 'grok-build-latest', to: 'grok-4.5' }
      ]
    )

    expect(mapping).toEqual({
      'grok-4.5': 'grok-4.5',
      'grok-latest': 'grok-4.5',
      'grok-4.5-latest': 'grok-4.5',
      'grok-build-latest': 'grok-4.5'
    })
  })

  it('grok 模型列表包含 Composer 默认项和兼容别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-composer-2.5-fast')
    expect(models).not.toContain('grok-composer')
    expect(models).toContain('composer-2.5')
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-3-flash'))
    expect(models.indexOf('gemini-3-pro-image')).toBeLessThan(models.indexOf('gemini-3.5-flash'))
  })

  it('gemini 模型列表不含已停用的 2.x 模型与 Antigravity 档位别名', () => {
    const models = getModelsByPlatform('gemini')

    for (const model of ['gemini-2.0-flash', 'gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-2.5-flash-image', 'gemini-3-pro-preview']) {
      expect(models).not.toContain(model)
    }
    for (const model of ['gemini-3.1-pro', 'gemini-3.1-pro-high', 'gemini-3.1-pro-low']) {
      expect(models).not.toContain(model)
    }
    expect(models).toContain('gemini-3.8-flash')
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('antigravity 模型列表包含 Gemini 3.1 Pro 通用别名', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-pro')
  })

  it('Gemini 4 Carbon exposes the official API model without invented aliases', () => {
    const models = getModelsByPlatform('gemini')
    expect(models).toContain('gemini-4-carbon')
    expect(models).not.toContain('gemini-4-carbon-preview')
    expect(getModelsByPlatform('antigravity')).toContain('gemini-4-carbon')
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-carbon', to: 'gemini-4-carbon' })
    ]))
    expect(buildModelMappingObject('whitelist', ['gemini-4-carbon'], [])).toEqual({
      'gemini-4-carbon': 'gemini-4-carbon'
    })
  })

  it('Gemini 4 Argon 在 gemini 与 antigravity 平台使用对应真实模型 ID 与预设映射', () => {
    expect(getModelsByPlatform('gemini')).toContain('gemini-4-argon')
    const agModels = getModelsByPlatform('antigravity')
    expect(agModels).not.toContain('gemini-4-argon')
    expect(agModels).toContain('gemini-4-argon-high')
    expect(agModels).toContain('gemini-4-argon-medium')
    expect(agModels).toContain('gemini-4-argon-low')
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-argon', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-high', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-medium', to: 'gemini-4-argon-medium' }),
      expect.objectContaining({ from: 'gemini-4-argon-low', to: 'gemini-4-argon-low' })
    ]))
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('combined mode retains a mapping when a whitelist entry has the same source', () => {
    expect(buildModelMappingObject('combined', ['gpt-latest'], [{ from: 'gpt-latest', to: 'deepseek-chat' }])).toEqual({
      'gpt-latest': 'deepseek-chat'
    })
  })

  it('split mapping keeps only identity entries in the whitelist after reopening', () => {
    expect(splitModelMappingObject({ 'gpt-latest': 'deepseek-chat', 'gpt-5.4': 'gpt-5.4' })).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }]
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })

  it('deleting a collapsed rewrite keeps the source as a whitelist identity', () => {
    const saved = buildModelMappingObject(
      'combined',
      ['gpt-5.6-sol'],
      [{ from: 'gpt-5.6-sol', to: 'gpt-6.1-sol' }]
    )
    expect(saved).toEqual({ 'gpt-5.6-sol': 'gpt-6.1-sol' })

    const reopened = splitModelMappingObject(saved)
    expect(reopened).toEqual({
      allowedModels: [],
      modelMappings: [{ from: 'gpt-5.6-sol', to: 'gpt-6.1-sol' }]
    })

    removeModelMappingEntry(reopened.modelMappings, 0, reopened.allowedModels)

    expect(reopened.modelMappings).toEqual([])
    expect(reopened.allowedModels).toEqual(['gpt-5.6-sol'])
    expect(buildModelMappingObject('combined', reopened.allowedModels, reopened.modelMappings)).toEqual({
      'gpt-5.6-sol': 'gpt-5.6-sol'
    })
  })

  it('deleting one rewrite restores only that source and keeps the other mapping', () => {
    const allowedModels = ['gpt-5.4']
    const modelMappings = [
      { from: 'gpt-5.6-sol', to: 'gpt-6.1-sol' },
      { from: 'gpt-latest', to: 'gpt-5.4' }
    ]

    removeModelMappingEntry(modelMappings, 0, allowedModels)

    expect(allowedModels).toEqual(['gpt-5.4', 'gpt-5.6-sol'])
    expect(modelMappings).toEqual([{ from: 'gpt-latest', to: 'gpt-5.4' }])
    expect(buildModelMappingObject('combined', allowedModels, modelMappings)).toEqual({
      'gpt-5.4': 'gpt-5.4',
      'gpt-5.6-sol': 'gpt-5.6-sol',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('does not duplicate a source that is already whitelisted, and skips blank or wildcard sources', () => {
    const allowedModels = [' gpt-5.6-sol ']
    const modelMappings = [
      { from: 'gpt-5.6-sol', to: 'gpt-6.1-sol' },
      { from: '  ', to: 'gpt-6.1-sol' },
      { from: 'claude-*', to: 'claude-sonnet-4-6' }
    ]

    removeModelMappingEntry(modelMappings, 0, allowedModels)
    removeModelMappingEntry(modelMappings, 0, allowedModels)
    removeModelMappingEntry(modelMappings, 0, allowedModels)

    expect(allowedModels).toEqual([' gpt-5.6-sol '])
    expect(modelMappings).toEqual([])
    expect(buildModelMappingObject('combined', ['gpt-5.6-sol'], [])).toEqual({
      'gpt-5.6-sol': 'gpt-5.6-sol'
    })
  })
})
