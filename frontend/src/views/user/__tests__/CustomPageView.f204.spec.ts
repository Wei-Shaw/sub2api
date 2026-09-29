import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CustomPageView from '../CustomPageView.vue'

const { f204AppStore, f204RefreshAuthTokens } = vi.hoisted(() => ({
  f204AppStore: {
    publicSettingsLoaded: true,
    cachedPublicSettings: { custom_menu_items: [{ id: 'docs', url: 'md:guide' }] },
  },
  f204RefreshAuthTokens: vi.fn(),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: 'docs' } }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
vi.mock('@/stores', () => ({ useAppStore: () => f204AppStore }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false, user: { id: 7 }, token: 'expired-token' }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/api/client', () => ({ buildApiUrl: (path: string) => `/api/v1${path}` }))
vi.mock('@/api/tokenRefresh', () => ({ refreshAuthTokens: f204RefreshAuthTokens }))

const f204Wrappers: ReturnType<typeof mount>[] = []

function f204MountPage() {
  const wrapper = mount(CustomPageView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
  })
  f204Wrappers.push(wrapper)
  return wrapper
}

describe('custom Markdown page with an expired access token (F2-04)', () => {
  beforeEach(() => {
    f204AppStore.cachedPublicSettings.custom_menu_items = [{ id: 'docs', url: 'md:guide' }]
    f204RefreshAuthTokens.mockReset()
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      disconnect() {}
    })
  })

  afterEach(() => {
    f204Wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.unstubAllGlobals()
  })

  it('refreshes the token once after a 401 and retries the page fetch', async () => {
    f204RefreshAuthTokens.mockResolvedValue({
      access_token: 'fresh-token', refresh_token: 'r2', expires_in: 1800, token_type: 'Bearer',
    })
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 401, text: async () => '' })
      .mockResolvedValueOnce({ ok: true, status: 200, text: async () => '# Guide' })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = f204MountPage()
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/pages/guide')
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer expired-token')
    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/pages/guide')
    expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe('Bearer fresh-token')
    expect(f204RefreshAuthTokens).toHaveBeenCalledTimes(1)
    expect(f204RefreshAuthTokens).toHaveBeenCalledWith({ failedAccessToken: 'expired-token' })
    expect(wrapper.get('.markdown-page-content h1').text()).toBe('Guide')
    expect(wrapper.html()).not.toContain('common.pageNotFound')
  })

  it('still shows page-not-found when the refresh itself fails', async () => {
    f204RefreshAuthTokens.mockRejectedValue(new Error('refresh failed'))
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 401, text: async () => '' })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = f204MountPage()
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(wrapper.html()).toContain('common.pageNotFound')
  })

  it('does not refresh for a genuine 404', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 404, text: async () => '' })
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = f204MountPage()
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(f204RefreshAuthTokens).not.toHaveBeenCalled()
    expect(wrapper.html()).toContain('common.pageNotFound')
  })
})
