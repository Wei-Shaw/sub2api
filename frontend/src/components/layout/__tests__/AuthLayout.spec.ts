import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AuthLayout from '@/components/layout/AuthLayout.vue'

const appState = vi.hoisted(() => ({
  siteName: 'DevRouter',
  siteLogo: '',
  publicSettingsLoaded: true,
  cachedPublicSettings: {
    site_subtitle: 'Subscription to API Conversion Platform',
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appState,
}))

describe('AuthLayout', () => {
  beforeEach(() => {
    appState.siteName = 'DevRouter'
    appState.siteLogo = ''
    appState.publicSettingsLoaded = true
    appState.cachedPublicSettings = {
      site_subtitle: 'Subscription to API Conversion Platform',
    }
    appState.fetchPublicSettings.mockReset()
  })

  it('renders the DevRouter auth shell with mist depth and no legacy subtitle', async () => {
    const wrapper = mount(AuthLayout, {
      slots: {
        default: '<div class="login-content">Login</div>',
        footer: '<span class="login-footer">Footer</span>',
      },
    })
    await flushPromises()

    expect(appState.fetchPublicSettings).toHaveBeenCalled()
    expect(wrapper.find('.auth-shell').exists()).toBe(true)
    expect(wrapper.find('.auth-grid').exists()).toBe(true)
    expect(wrapper.find('.auth-mist-violet').exists()).toBe(true)
    expect(wrapper.find('.auth-mist-cyan').exists()).toBe(true)
    expect(wrapper.find('.auth-brand-mark').text()).toBe('DR')
    expect(wrapper.find('.auth-brand-name').classes()).toEqual(
      expect.arrayContaining(['font-extrabold', 'font-[Inter,Geist,system-ui,sans-serif]'])
    )
    expect(wrapper.text()).toContain('DevRouter')
    expect(wrapper.text()).not.toContain('Subscription to API Conversion Platform')
    expect(wrapper.find('.auth-card').classes()).toEqual(
      expect.arrayContaining(['rounded-2xl', 'border', 'border-slate-200/70', 'bg-white/85', 'backdrop-blur-2xl'])
    )
    expect(wrapper.find('.auth-card').classes().join(' ')).toContain('rgba(15,23,42,0.05)')
    expect(wrapper.find('.auth-card').classes().join(' ')).toContain('rgba(0,0,0,0.02)')
  })
})
