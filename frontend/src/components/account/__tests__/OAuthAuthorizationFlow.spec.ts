import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: vi.fn(),
    showError: vi.fn()
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: { value: false },
    copyToClipboard: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

describe('OAuthAuthorizationFlow', () => {
  it('extracts code, state, and callback metadata from a full Kiro callback URL', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'kiro',
        authUrl: 'https://example.com/authorize',
        sessionId: 'session-1'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    const textarea = wrapper.get('textarea')
    await textarea.setValue('http://localhost:49153/oauth/callback?code=abc123&state=state456&login_option=github')
    await nextTick()

    expect((textarea.element as HTMLTextAreaElement).value).toBe('abc123')
    expect((wrapper.vm as any).oauthState).toBe('state456')
    expect((wrapper.vm as any).oauthCallbackPath).toBe('/oauth/callback')
    expect((wrapper.vm as any).oauthLoginOption).toBe('github')
  })
})
