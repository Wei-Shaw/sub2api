import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OAuthCallbackView from '@/views/auth/OAuthCallbackView.vue'

const { routeState, showErrorMock, copyToClipboardMock } = vi.hoisted(() => ({
  routeState: {
    query: {} as Record<string, unknown>,
  },
  showErrorMock: vi.fn(),
  copyToClipboardMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: (...args: any[]) => copyToClipboardMock(...args),
  }),
}))

describe('OAuthCallbackView', () => {
  beforeEach(() => {
    routeState.query = {}
    showErrorMock.mockReset()
    copyToClipboardMock.mockReset()
  })

  it('renders localized callback copy actions', () => {
    routeState.query = {
      code: 'oauth-code',
      state: 'oauth-state',
    }

    const wrapper = mount(OAuthCallbackView)

    expect(wrapper.text()).toContain('auth.oauth.callbackTitle')
    expect(wrapper.text()).toContain('auth.oauth.callbackHint')
    expect(wrapper.text()).toContain('common.copy')
    expect(wrapper.find('input[value="oauth-code"]').exists()).toBe(true)
    expect(wrapper.find('input[value="oauth-state"]').exists()).toBe(true)
  })

  it('sends callback errors to toast instead of rendering inline red text', () => {
    routeState.query = {
      error: 'oauth failed',
    }

    const wrapper = mount(OAuthCallbackView)

    expect(showErrorMock).toHaveBeenCalledWith('oauth failed')
    expect(wrapper.text()).not.toContain('oauth failed')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })
})
