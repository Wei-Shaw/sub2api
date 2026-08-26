import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)
const showWarning = vi.fn()

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => (key === 'common.copy' ? '复制' : key)
    })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

const { syncUpstreamModels, syncUpstreamModelsPreview } = vi.hoisted(() => ({
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn()
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels,
    syncUpstreamModelsPreview
  }
}))

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

function mountSelector() {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai'
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
    showWarning.mockClear()
    syncUpstreamModels.mockReset()
    syncUpstreamModelsPreview.mockReset()
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

  it('loads the live Cursor picker into dropdown options without selecting them', async () => {
    syncUpstreamModels.mockResolvedValue({
      models: ['default', 'claude-opus-5', 'gpt-5.6-sol']
    })

    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'cursor',
        accountId: 51
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })
    await flushPromises()

    expect(syncUpstreamModels).toHaveBeenCalledWith(51)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('catalog-loaded')?.[0]).toEqual([['default', 'claude-opus-5', 'gpt-5.6-sol']])

    await wrapper.get('div.cursor-pointer').trigger('click')
    findModelRow(wrapper, 'claude-opus-5')
    findModelRow(wrapper, 'gpt-5.6-sol')
  })

  it('warns when Sync latest supported models uses the static Cursor fallback', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'cursor'
      },
      global: {
        stubs: {
          ModelIcon: true
        }
      }
    })

    const buttons = wrapper.findAll('button')
    const fillButton = buttons.find(button => button.text() === 'admin.accounts.fillRelatedModels')
    expect(fillButton).toBeDefined()
    await fillButton!.trigger('click')
    await flushPromises()

    const selected = wrapper.emitted('update:modelValue')?.[0]?.[0] as string[]
    expect(selected).toContain('gpt-5.4-mini')
    expect(selected).toContain('kimi-k2.7-code')
    expect(selected.length).toBeGreaterThan(6)
    expect(showWarning).toHaveBeenCalledWith('admin.accounts.cursorStaticFallbackUsed')
  })
})
