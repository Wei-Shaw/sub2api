import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed } from 'vue'

const { listModelCatalog, authState } = vi.hoisted(() => ({
  listModelCatalog: vi.fn(),
  authState: { isAdmin: false }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/api/admin/modelCatalog', () => ({
  list: listModelCatalog
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

import {
  __resetCatalogModelsCacheForTest,
  buildModelMappingObject,
  claudeModels,
  ensureCatalogModelsLoaded,
  getCatalogModelsByPlatform,
  getModelsByPlatform,
  getPresetMappingsByPlatform,
  reloadCatalogModels,
  splitModelMappingObject
} from '../useModelWhitelist'

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

function catalogItem(model_id: string, platform: string, enabled = true) {
  return { id: model_id.length, model_id, platform, enabled }
}

beforeEach(() => {
  authState.isAdmin = false
  listModelCatalog.mockReset()
  listModelCatalog.mockResolvedValue({ items: [] })
  __resetCatalogModelsCacheForTest()
})

afterEach(() => {
  __resetCatalogModelsCacheForTest()
})

describe('useModelWhitelist', () => {
  it('openai 模型列表包含 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('codex-auto-review')
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
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it('Claude 模型列表包含 Claude 5 系列', () => {
    const models = getModelsByPlatform('anthropic')
    for (const id of [
      'claude-opus-5', 'claude-opus-5-5',
      'claude-sonnet-5', 'claude-sonnet-5-5',
      'claude-haiku-5-5',
      'claude-fable-5-1', 'claude-fable-5-2'
    ]) {
      expect(models).toContain(id)
    }
    // antigravity 列表不受影响
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-opus-5-5')
  })

  it('Anthropic 预设映射包含 Claude 5 系列身份映射，antigravity/bedrock 预设不变', () => {
    const presets = getPresetMappingsByPlatform('anthropic')
    for (const [label, id] of [
      ['Opus 5.5', 'claude-opus-5-5'],
      ['Sonnet 5.5', 'claude-sonnet-5-5'],
      ['Haiku 5.5', 'claude-haiku-5-5'],
      ['Fable 5.2', 'claude-fable-5-2']
    ]) {
      expect(presets).toContainEqual(expect.objectContaining({ label, from: id, to: id }))
    }
    expect(getPresetMappingsByPlatform('antigravity').some(p => p.from === 'claude-opus-5-5')).toBe(false)
    expect(getPresetMappingsByPlatform('bedrock').some(p => p.from === 'claude-opus-5-5')).toBe(false)
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
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

  describe('模型目录合并', () => {
    it('非管理员不请求模型目录，返回硬编码列表', async () => {
      authState.isAdmin = false
      expect(getModelsByPlatform('anthropic')).toBe(claudeModels)
      await flush()
      expect(listModelCatalog).not.toHaveBeenCalled()
    })

    it('管理员懒加载一次，按平台去重并追加在硬编码列表之后', async () => {
      authState.isAdmin = true
      listModelCatalog.mockResolvedValue({
        items: [
          catalogItem('claude-opus-6', 'anthropic'),
          catalogItem('claude-opus-4-8', 'anthropic'), // 已在硬编码列表中
          catalogItem('claude-opus-6', 'anthropic'), // 重复
          catalogItem('gpt-6', 'openai'),
          catalogItem('claude-disabled', 'anthropic', false)
        ]
      })

      // 首次调用返回硬编码列表并触发加载
      expect(getModelsByPlatform('anthropic')).toBe(claudeModels)
      getModelsByPlatform('openai')
      await flush()

      expect(listModelCatalog).toHaveBeenCalledTimes(1)
      expect(listModelCatalog).toHaveBeenCalledWith({ enabled: true })

      const anthropic = getModelsByPlatform('anthropic')
      expect(anthropic.slice(0, claudeModels.length)).toEqual(claudeModels)
      expect(anthropic.slice(claudeModels.length)).toEqual(['claude-opus-6'])
      expect(anthropic.filter(m => m === 'claude-opus-4-8')).toHaveLength(1)
      expect(anthropic).not.toContain('claude-disabled')
      expect(anthropic).not.toContain('gpt-6')

      // claude / bedrock / 默认分支 共享 anthropic 目录
      expect(getModelsByPlatform('claude')).toContain('claude-opus-6')
      expect(getModelsByPlatform('bedrock')).toContain('claude-opus-6')
      expect(getModelsByPlatform('openai')).toContain('gpt-6')
      // 纯厂商列表不合并
      expect(getModelsByPlatform('deepseek')).not.toContain('claude-opus-6')
      expect(getCatalogModelsByPlatform('deepseek')).toEqual([])

      getModelsByPlatform('gemini')
      await flush()
      expect(listModelCatalog).toHaveBeenCalledTimes(1)
    })

    it('computed 调用方在目录加载完成后自动更新', async () => {
      authState.isAdmin = true
      listModelCatalog.mockResolvedValue({ items: [catalogItem('gemini-9-pro', 'gemini')] })

      const models = computed(() => getModelsByPlatform('gemini'))
      expect(models.value).not.toContain('gemini-9-pro')
      await flush()
      expect(models.value).toContain('gemini-9-pro')
    })

    it('请求失败时静默回退到硬编码列表', async () => {
      authState.isAdmin = true
      listModelCatalog.mockRejectedValue({ status: 403, message: 'forbidden' })

      await expect(ensureCatalogModelsLoaded()).resolves.toBeUndefined()
      expect(getModelsByPlatform('anthropic')).toBe(claudeModels)
    })

    it('reloadCatalogModels 会重新请求并替换缓存', async () => {
      authState.isAdmin = true
      listModelCatalog.mockResolvedValueOnce({ items: [catalogItem('claude-old', 'anthropic')] })
      await ensureCatalogModelsLoaded()
      expect(getModelsByPlatform('anthropic')).toContain('claude-old')

      listModelCatalog.mockResolvedValueOnce({ items: [catalogItem('claude-new', 'anthropic')] })
      await reloadCatalogModels()

      expect(listModelCatalog).toHaveBeenCalledTimes(2)
      const models = getModelsByPlatform('anthropic')
      expect(models).toContain('claude-new')
      expect(models).not.toContain('claude-old')
    })
  })
})
