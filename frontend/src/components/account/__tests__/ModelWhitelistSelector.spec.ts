import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import {
  BUILTIN_PLATFORM_CATALOG,
  resetPlatformCatalog,
  setPlatformCatalog
} from '@/constants/platformCatalog'

const {
  copyToClipboard,
  showError,
  showSuccess,
  showInfo,
  showWarning,
  syncUpstreamModels,
  syncUpstreamModelsPreview
} = vi.hoisted(() => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  showWarning: vi.fn(),
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => key === 'common.copy' ? '复制' : key === 'admin.accounts.modelMappingConflict' ? `Model mapping conflict: ${params?.from} → ${params?.to}` : key
    })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo,
    showWarning
  })
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels,
    syncUpstreamModelsPreview
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

function mountSelector(props: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      ...props,
    },
    global: {
      stubs: {
        ModelIcon: true
      }
    }
  })
}

function findModelRow(wrapper: ReturnType<typeof mountSelector>, modelId: string) {
  const row = wrapper
    .findAll('[data-testid="model-option"]')
    .find(candidate => candidate.text().includes(modelId))

  if (!row) {
    throw new Error(`Model row not found: ${modelId}`)
  }

  return row
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    copyToClipboard.mockClear()
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    syncUpstreamModels.mockReset()
    syncUpstreamModelsPreview.mockReset()
  })

  afterEach(() => {
    resetPlatformCatalog()
  })

  it.each(['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'command_code', 'cline'])(
    'supports upstream sync for %s saved accounts and creation previews',
    (platform) => {
      const wrappers = [
        mountSelector({ platform, accountId: 46 }),
        mountSelector({ platform, syncCredentials: { platform, type: 'apikey', api_key: 'test-key' } })
      ]
      for (const wrapper of wrappers) {
        expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(true)
        wrapper.unmount()
      }
    }
  )

  it.each(['typesafe', 'unregistered'])(
    'hides upstream sync for unsupported %s saved accounts and creation previews',
    (platform) => {
      const wrappers = [
        mountSelector({ platform, accountId: 46 }),
        mountSelector({ platform, syncCredentials: { platform, type: 'apikey', api_key: 'test-key' } })
      ]
      for (const wrapper of wrappers) {
        expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(false)
        wrapper.unmount()
      }
      expect(syncUpstreamModels).not.toHaveBeenCalled()
      expect(syncUpstreamModelsPreview).not.toHaveBeenCalled()
    }
  )

  it('requires a supported request builder for newly registered platforms', async () => {
    const wrapper = mountSelector({ platform: 'acme_router', accountId: 46 })
    const platforms = [
      ...BUILTIN_PLATFORM_CATALOG.platforms,
      { id: 'acme_router', display_name: 'Acme Router', gateway: 'openai' as const, cn_provider: false }
    ]
    setPlatformCatalog({ ...BUILTIN_PLATFORM_CATALOG, platforms })
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(false)

    setPlatformCatalog({
      ...BUILTIN_PLATFORM_CATALOG,
      platforms: platforms.map(spec => spec.id === 'acme_router'
        ? { ...spec, multi_protocol: { default_mode: 'default', routing: 'by_inbound', modes: [] } }
        : spec)
    })
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.syncUpstreamModels')).toBe(true)
    wrapper.unmount()
  })

  it('rejects a custom whitelist model that is already mapped to a different target', async () => {
    const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue(' gpt-latest ')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(showInfo).toHaveBeenCalledWith(expect.stringContaining('gpt-latest → deepseek-chat'))
  })

  it('keeps the existing duplicate identity warning before checking mappings', async () => {
    const wrapper = mountSelector({ modelValue: ['gpt-latest'], modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('gpt-latest')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(showInfo).toHaveBeenCalledWith('admin.accounts.modelExists')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('allows matching identity mapping as a whitelist model', async () => {
    const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'gpt-latest' }] })
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('gpt-latest')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-latest']]])
  })

  it('still allows custom models without a mapping prop', async () => {
    const wrapper = mountSelector()
    await wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]').setValue('custom-model')
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.addModel')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[['custom-model']]])
  })

  it('copies a model ID without selecting the model', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')

    const copyButton = row.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('复制 gpt-5.6-sol')

    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the existing model selection behavior', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')
    await row.get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.6-sol']]])
    expect(copyToClipboard).not.toHaveBeenCalled()
  })

  it('warns when model IDs sync but capability metadata is incomplete', async () => {
    syncUpstreamModels.mockResolvedValue({
      models: ['x-preview-f-free'],
      warnings: [
        {
          code: 'upstream_model_metadata_incomplete',
          message: 'Model IDs were synced, but capability metadata could not be updated.'
        }
      ]
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 46
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })

    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(findModelRow(wrapper, 'x-preview-f-free').exists()).toBe(true)
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataIncomplete')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('shows success and a partial warning when some capabilities were saved', async () => {
    syncUpstreamModels.mockResolvedValue({
      models: ['gpt-6-astra', 'gpt-image-2'],
      warnings: [
        {
          code: 'upstream_model_metadata_partial',
          message: 'Some model capabilities were saved; remaining models are still incomplete.'
        }
      ]
    })
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'openai',
        accountId: 46
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })

    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(findModelRow(wrapper, 'gpt-6-astra').exists()).toBe(true)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.upstreamModelCandidatesLoaded')
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsMetadataPartial')
  })

  it('reports a successful preview so account creation can persist metadata', async () => {
    syncUpstreamModelsPreview.mockResolvedValue({
      models: ['x-preview-f-free'],
      metadata: {
        'x-preview-f-free': {
          id: 'x-preview-f-free',
          reasoning: true,
          supported_reasoning_levels: ['low', 'high', 'max'],
        },
      },
    })
    const wrapper = mountSelector({
      syncCredentials: {
        platform: 'openai',
        type: 'apikey',
        base_url: 'https://opencode.ai/zen/v1',
        api_key: 'test-key',
      },
    })
    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')

    expect(syncButton).toBeDefined()
    await syncButton?.trigger('click')
    await flushPromises()

    expect(syncUpstreamModelsPreview).toHaveBeenCalledOnce()
    expect(wrapper.emitted('upstream-synced')).toEqual([[]])
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(findModelRow(wrapper, 'x-preview-f-free').exists()).toBe(true)
  })

  it('shows the upstream sync button for OpenCode Go create-account credentials', () => {
    const wrapper = mountSelector({
      platform: 'opencode_go',
      syncCredentials: {
        platform: 'opencode_go',
        type: 'apikey',
        base_url: 'https://opencode.ai/zen/go/v1',
        api_key: 'sk-test',
      },
    })
    const syncButton = wrapper
      .findAll('button')
      .find(button => button.text() === 'admin.accounts.syncUpstreamModels')

    expect(syncButton).toBeDefined()
    expect(syncButton?.exists()).toBe(true)
  })

  it('loads over 200 upstream candidates without selecting them or showing unrelated built-ins', async () => {
    const models = [
      ...Array.from({ length: 120 }, (_, index) => `gpt-upstream-${index}`),
      ...Array.from({ length: 100 }, (_, index) => `gemini-upstream-${index}`),
      'claude-upstream-only'
    ]
    syncUpstreamModels.mockResolvedValue({ models })
    const wrapper = mountSelector({ platform: 'gemini', accountId: 33, modelValue: ['saved-custom-model'] })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(222)
    expect(wrapper.find('[data-testid="upstream-model-candidates"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="model-option"]').some(row => row.text().includes('gemini-2.0-flash'))).toBe(false)

    await wrapper.get('[data-testid="model-search"]').setValue('  GeMiNi  ')
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(100)
    expect(wrapper.findAll('[data-testid="selected-model"]')).toHaveLength(0)
    await wrapper.get('[data-testid="model-search"]').setValue('GPT')
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(120)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('adds and removes only filtered results while keeping other selected models', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gpt-upstream', 'gemini-upstream-a', 'gemini-upstream-b'] })
    const wrapper = mountSelector({ accountId: 33, modelValue: ['gpt-upstream', 'manual-model', 'gemini-upstream-a'] })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="model-search"]').setValue('gemini')
    await wrapper.get('[data-testid="select-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[
      'gpt-upstream', 'manual-model', 'gemini-upstream-a', 'gemini-upstream-b'
    ]])

    await wrapper.setProps({ modelValue: ['gpt-upstream', 'manual-model', 'gemini-upstream-a', 'gemini-upstream-b'] })
    await wrapper.get('[data-testid="remove-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([['gpt-upstream', 'manual-model']])
  })

  it('supports single selection and deselection of custom upstream IDs', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gemini-custom-future'] })
    const wrapper = mountSelector({ platform: 'gemini', accountId: 33 })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    const button = findModelRow(wrapper, 'gemini-custom-future').get('[data-testid="select-model"]')
    expect(button.attributes('aria-checked')).toBe('false')
    await button.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([['gemini-custom-future']])
    await wrapper.setProps({ modelValue: ['gemini-custom-future'] })
    expect(button.attributes('aria-checked')).toBe('true')
    await button.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[]])
  })

  it('keeps saved models that are no longer in the upstream list searchable and removable', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gemini-current'] })
    const wrapper = mountSelector({ platform: 'gemini', accountId: 33, modelValue: ['gemini-retired', 'gpt-keep'] })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="model-search"]').setValue('retired')
    expect(findModelRow(wrapper, 'gemini-retired').text()).toContain('admin.accounts.selectedModelNotInUpstream')
    await wrapper.get('[data-testid="remove-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([['gpt-keep']])
  })

  it('trims and deduplicates upstream candidate IDs without normalizing their case', async () => {
    syncUpstreamModels.mockResolvedValue({ models: [' Gemini-Custom ', 'Gemini-Custom', '', '  ', 'gemini-other'] })
    const wrapper = mountSelector({ accountId: 33 })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(2)
    await wrapper.get('[data-testid="model-search"]').setValue('gemini')
    await wrapper.get('[data-testid="select-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([['Gemini-Custom', 'gemini-other']])
  })

  it('preserves custom mappings when selecting a filtered batch', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gemini-alias', 'gemini-plain'] })
    const wrapper = mountSelector({
      accountId: 33,
      modelValue: ['gpt-keep'],
      modelMappings: [{ from: 'gemini-alias', to: 'gemini-real-target' }]
    })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="model-search"]').setValue('gemini')
    expect(findModelRow(wrapper, 'gemini-alias').get('[data-testid="select-model"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="select-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([['gpt-keep', 'gemini-plain']])
    expect(wrapper.props('modelMappings')).toEqual([{ from: 'gemini-alias', to: 'gemini-real-target' }])
  })

  it.each(['failure', 'empty'])('does not discard previous choices or candidates on sync %s', async (outcome) => {
    syncUpstreamModels.mockResolvedValueOnce({ models: ['gemini-existing-candidate'] })
    const wrapper = mountSelector({ accountId: 33, modelValue: ['saved-model'] })
    const button = wrapper.findAll('button').find(item => item.text() === 'admin.accounts.syncUpstreamModels')!
    await button.trigger('click')
    await flushPromises()
    if (outcome === 'failure') {
      syncUpstreamModels.mockRejectedValueOnce(new Error('Network unavailable'))
    } else {
      syncUpstreamModels.mockResolvedValueOnce({ models: [] })
    }
    await button.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(findModelRow(wrapper, 'gemini-existing-candidate').exists()).toBe(true)
    expect(findModelRow(wrapper, 'saved-model').exists()).toBe(true)
    if (outcome === 'failure') {
      expect(showError).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsError')
    } else {
      expect(showInfo).toHaveBeenCalledWith('admin.accounts.syncUpstreamModelsEmpty')
    }
  })

  it('disables batch actions when nothing matches and keeps selection intact', async () => {
    const wrapper = mountSelector({ modelValue: ['custom-kept'] })
    await wrapper.get('[data-testid="model-search"]').setValue('nothing-matches-this')
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="select-filtered-models"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="remove-filtered-models"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="select-filtered-models"]').trigger('click')
    await wrapper.get('[data-testid="remove-filtered-models"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('resets candidates when switching accounts without deleting saved selections', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gemini-old-account'] })
    const wrapper = mountSelector({ accountId: 33, modelValue: ['custom-kept'] })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="model-search"]').setValue('gemini')
    await wrapper.setProps({ accountId: 34 })
    expect(wrapper.find('[data-testid="upstream-model-candidates"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="model-search"]').element).toHaveProperty('value', '')
    expect(wrapper.findAll('[data-testid="model-option"]').some(row => row.text().includes('gemini-old-account'))).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('ignores a stale preview response when the upstream credentials change', async () => {
    let resolveSync: ((result: { models: string[] }) => void) | undefined
    syncUpstreamModelsPreview.mockReturnValue(new Promise<{ models: string[] }>(resolve => { resolveSync = resolve }))
    const credentials = { platform: 'gemini', type: 'apikey', base_url: 'https://old.example.test', api_key: 'test-key' }
    const wrapper = mountSelector({ platform: 'gemini', syncCredentials: credentials })
    await wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!.trigger('click')
    await wrapper.setProps({ syncCredentials: { ...credentials, base_url: 'https://new.example.test' } })
    resolveSync?.({ models: ['gemini-stale-response'] })
    await flushPromises()
    expect(wrapper.find('[data-testid="upstream-model-candidates"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="model-option"]').some(row => row.text().includes('gemini-stale-response'))).toBe(false)
    expect(wrapper.emitted('upstream-synced')).toBeUndefined()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the newest sync loading state when an older request finishes', async () => {
    let resolveFirst: ((result: { models: string[] }) => void) | undefined
    let resolveSecond: ((result: { models: string[] }) => void) | undefined
    syncUpstreamModels
      .mockReturnValueOnce(new Promise<{ models: string[] }>(resolve => { resolveFirst = resolve }))
      .mockReturnValueOnce(new Promise<{ models: string[] }>(resolve => { resolveSecond = resolve }))
    const wrapper = mountSelector({ accountId: 33 })
    const syncButton = () => wrapper.findAll('button').find(button =>
      ['admin.accounts.syncUpstreamModels', 'admin.accounts.syncUpstreamModelsLoading'].includes(button.text())
    )!
    await syncButton().trigger('click')
    await wrapper.setProps({ accountId: 34 })
    await syncButton().trigger('click')
    resolveFirst?.({ models: ['old-model'] })
    await flushPromises()
    expect(syncButton().attributes('disabled')).toBeDefined()
    resolveSecond?.({ models: ['new-model'] })
    await flushPromises()
    expect(syncButton().attributes('disabled')).toBeUndefined()
    expect(findModelRow(wrapper, 'new-model').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="model-option"]').some(row => row.text().includes('old-model'))).toBe(false)
  })

  it('keeps search results when closing and reopening the candidate list', async () => {
    const wrapper = mountSelector()
    await wrapper.get('[data-testid="model-search"]').setValue('GPT')
    await wrapper.get('div.cursor-pointer').trigger('click')
    expect(wrapper.find('[data-testid="model-option"]').exists()).toBe(false)
    await wrapper.get('div.cursor-pointer').trigger('click')
    expect(wrapper.get('[data-testid="model-search"]').element).toHaveProperty('value', 'GPT')
    expect(wrapper.findAll('[data-testid="model-option"]').every(row => row.text().toLowerCase().includes('gpt'))).toBe(true)
  })
})
