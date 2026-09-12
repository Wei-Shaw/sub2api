import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import KeyModelsModal from '../KeyModelsModal.vue'

const { listModels, copyToClipboard } = vi.hoisted(() => ({
  listModels: vi.fn(),
  copyToClipboard: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.close': 'Close',
  'keys.modelsModal.title': 'Available Models - {name}',
  'keys.modelsModal.description': 'Live model list',
  'keys.modelsModal.loading': 'Loading models...',
  'keys.modelsModal.searchPlaceholder': 'Search model IDs...',
  'keys.modelsModal.count': '{count} models',
  'keys.modelsModal.copy': 'Copy model ID',
  'keys.modelsModal.empty': 'No available models',
  'keys.modelsModal.noMatches': 'No matching models',
  'keys.modelsModal.failedToLoad': 'Unable to load models',
  'keys.modelsModal.retry': 'Retry',
}

vi.mock('@/api', () => ({
  keysAPI: {
    listModels,
  },
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      const message = messages[key] ?? key
      return Object.entries(params ?? {}).reduce(
        (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
        message
      )
    },
  }),
}))

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: `
    <div v-if="show">
      <div data-testid="dialog-title">{{ title }}</div>
      <slot />
      <slot name="footer" />
    </div>
  `,
}

const mountModal = () =>
  mount(KeyModelsModal, {
    props: {
      show: true,
      apiKey: 'sk-test-key',
      keyName: 'test-key',
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ModelIcon: true,
        Icon: true,
      },
    },
  })

describe('KeyModelsModal', () => {
  beforeEach(() => {
    listModels.mockReset()
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)
  })

  it('loads, searches, and copies effective model IDs', async () => {
    listModels.mockResolvedValue([
      { id: 'gpt-5.6', display_name: 'GPT-5.6' },
      { id: 'gpt-5-mini' },
    ])

    const wrapper = mountModal()
    await flushPromises()

    expect(listModels).toHaveBeenCalledWith(
      'sk-test-key',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(wrapper.get('[data-testid="dialog-title"]').text()).toBe(
      'Available Models - test-key'
    )
    expect(wrapper.findAll('[data-testid="model-row"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('2 models')

    await wrapper.get('[data-testid="model-search"]').setValue('mini')

    expect(wrapper.findAll('[data-testid="model-row"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('gpt-5-mini')
    expect(wrapper.text()).not.toContain('gpt-5.6')

    await wrapper.get('[data-testid="copy-model-id"]').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5-mini')
  })

  it('shows a generic error and can retry', async () => {
    listModels
      .mockRejectedValueOnce(new Error('private upstream detail'))
      .mockResolvedValueOnce([{ id: 'gpt-5.6' }])

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('Unable to load models')
    expect(wrapper.text()).not.toContain('private upstream detail')

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('gpt-5.6')
  })

  it('shows a distinct empty search result', async () => {
    listModels.mockResolvedValue([{ id: 'gpt-5.6' }])

    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="model-search"]').setValue('claude')

    expect(wrapper.text()).toContain('No matching models')
  })
})
