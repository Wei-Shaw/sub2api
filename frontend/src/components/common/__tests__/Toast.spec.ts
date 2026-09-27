import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Toast from '../Toast.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    toasts: [{ id: 't1', type: 'info', message: 'Saved' }],
    hideToast: vi.fn()
  })
}))

describe('Toast', () => {
  it('labels the close button with the localized close text', () => {
    const wrapper = mount(Toast, { global: { stubs: { teleport: true, Icon: true } } })

    expect(wrapper.get('button').attributes('aria-label')).toBe('common.close')
  })
})
