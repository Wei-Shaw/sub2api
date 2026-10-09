import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import CustomPageView from '../CustomPageView.vue'

const route = reactive({ params: { id: 'old' } })
const fetchPage = vi.fn()
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({
  publicSettingsLoaded: true,
  cachedPublicSettings: { custom_menu_items: [
    { id: 'old', url: 'md:old' }, { id: 'new', url: 'md:new' },
    { id: 'external', url: 'https://example.com/docs' }
  ] }
}) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false, user: { id: 1 }, token: 'test' }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/api/client', () => ({ buildApiUrl: (path: string) => `/api/v1${path}` }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
enableAutoUnmount(afterEach)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const response = (markdown: string) => ({ ok: true, text: async () => markdown }) as Response
const mountPage = () => mount(CustomPageView, { global: { stubs: { Icon: true } } })

beforeEach(() => {
  route.params.id = 'old'
  fetchPage.mockReset()
  vi.stubGlobal('fetch', fetchPage)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => vi.unstubAllGlobals())

describe('custom Markdown page request ordering', () => {
  it.each(['success', 'http error', 'network error'])('ignores an old %s after switching pages', async (outcome) => {
    const old = deferred<Response>()
    fetchPage.mockReturnValueOnce(old.promise).mockResolvedValueOnce(response('# New page'))
    const wrapper = mountPage()
    route.params.id = 'new'
    await flushPromises()
    expect(wrapper.get('.markdown-page-content h1').text()).toBe('New page')
    if (outcome === 'network error') old.reject(new Error('offline'))
    else old.resolve(outcome === 'http error' ? { ok: false } as Response : response('# Old page'))
    await flushPromises()
    expect(wrapper.get('.markdown-page-content h1').text()).toBe('New page')
    expect(wrapper.get('.toc-item').text()).toBe('New page')
  })

  it('ignores an old body that finishes reading after the new page', async () => {
    const body = deferred<string>()
    fetchPage.mockResolvedValueOnce({ ok: true, text: () => body.promise })
      .mockResolvedValueOnce(response('# New page'))
    const wrapper = mountPage()
    await flushPromises()
    route.params.id = 'new'
    await flushPromises()
    body.resolve('# Old page')
    await flushPromises()
    expect(wrapper.get('.markdown-page-content h1').text()).toBe('New page')
  })

  it('keeps the current loading state when the old request completes', async () => {
    const old = deferred<Response>()
    const current = deferred<Response>()
    fetchPage.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = mountPage()
    route.params.id = 'new'
    await nextTick()
    old.resolve(response('# Old page'))
    await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
    expect(wrapper.find('.markdown-page-content').exists()).toBe(false)
    current.resolve(response('# New page'))
    await flushPromises()
    expect(wrapper.get('.markdown-page-content h1').text()).toBe('New page')
  })

  it('shows an embedded page immediately after leaving a pending Markdown page', async () => {
    const old = deferred<Response>()
    fetchPage.mockReturnValueOnce(old.promise)
    const wrapper = mountPage()
    route.params.id = 'external'
    await nextTick()
    expect(wrapper.get('iframe').attributes('src')).toContain('https://example.com/docs')
    old.resolve(response('# Old page'))
    await flushPromises()
    expect(wrapper.find('.markdown-page-content').exists()).toBe(false)
    expect(wrapper.get('iframe').attributes('src')).toContain('https://example.com/docs')
  })
})
