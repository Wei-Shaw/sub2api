import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AvailableModelsModal from '../AvailableModelsModal.vue'

const { copyToClipboard } = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'common.close': 'Close',
      'keys.availableModelsTitle': 'Available Group Models',
      'keys.availableModelsDescription': 'Description',
      'keys.availableModelsCount': '1 models',
      'keys.availableModelsCopy': 'Copy model name',
      'keys.availableModelsCopied': 'Model name copied',
    })[key] ?? key,
  }),
}))

const BaseDialogStub = {
  props: ['show', 'title'],
  template: `
    <div v-if="show">
      <h2>{{ title }}</h2>
      <slot />
      <slot name="footer" />
    </div>
  `,
}

describe('AvailableModelsModal', () => {
  beforeEach(() => {
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('copies the model name with the shared clipboard composable', async () => {
    const wrapper = mount(AvailableModelsModal, {
      props: {
        show: true,
        models: ['deepseek-v4-flash'],
        loading: false,
        error: '',
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Icon: true,
        },
      },
    })

    await wrapper.get('[data-test="copy-model"]').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith('deepseek-v4-flash', 'Model name copied')
    expect(wrapper.get('[data-test="copy-model"]').attributes('aria-label')).toBe('Model name copied')
  })
})
