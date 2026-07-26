import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const { copyToClipboard, getModelsListCandidates, syncUpstreamModels } = vi.hoisted(() => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
  getModelsListCandidates: vi.fn(),
  syncUpstreamModels: vi.fn()
}))

const appStore = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore
}))

vi.mock('@/api/admin/groups', () => ({
  getModelsListCandidates
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: {
    syncUpstreamModels
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'common.copy') return '复制'
        return params ? `${key}:${JSON.stringify(params)}` : key
      }
    })
  }
})

vi.mock('@/components/common/ModelIcon.vue', () => ({
  default: { template: '<span />' }
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { template: '<span />' }
}))

function mountSelector() {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai'
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
    getModelsListCandidates.mockReset().mockResolvedValue(['gpt-5.6-sol', 'claude-opus-5'])
    syncUpstreamModels.mockReset().mockResolvedValue({ models: ['claude-opus-5', 'customer-custom-model'] })
    appStore.showError.mockReset()
    appStore.showSuccess.mockReset()
    appStore.showInfo.mockReset()
  })

  it('copies a published model ID without selecting the model', async () => {
    const wrapper = mountSelector()
    await flushPromises()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')
    const copyButton = row.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('复制 gpt-5.6-sol')

    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the existing model selection behavior for published models', async () => {
    const wrapper = mountSelector()
    await flushPromises()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')
    await row.get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.6-sol']]])
    expect(copyToClipboard).not.toHaveBeenCalled()
  })

  it('does not render a custom model input outside the catalog selector', async () => {
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'anthropic'
      }
    })
    await flushPromises()

    expect(wrapper.find('input[placeholder="admin.accounts.enterCustomModelName"]').exists()).toBe(false)
  })

  it('filters upstream sync results through catalog candidates before emitting', async () => {
    const onUpdate = vi.fn()
    const wrapper = mount(ModelWhitelistSelector, {
      props: {
        modelValue: [],
        platform: 'anthropic',
        accountId: 42,
        'onUpdate:modelValue': onUpdate
      }
    })
    await flushPromises()

    const syncButton = wrapper.findAll('button').find(button => button.text().includes('admin.accounts.syncUpstreamModels'))
    expect(syncButton).toBeDefined()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(onUpdate).toHaveBeenCalledWith(['claude-opus-5'])
  })

  it('removes preloaded custom selections after catalog candidates load', async () => {
    const onUpdate = vi.fn()
    mount(ModelWhitelistSelector, {
      props: {
        modelValue: ['customer-custom-model', 'claude-opus-5'],
        platform: 'anthropic',
        'onUpdate:modelValue': onUpdate
      }
    })

    await flushPromises()

    expect(onUpdate).toHaveBeenCalledWith(['claude-opus-5'])
  })
})
