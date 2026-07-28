import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import UserDashboardQuickAccess from '../UserDashboardQuickAccess.vue'

// Real shape: keysAPI.list(page, pageSize, filters, options) -> PaginatedResponse<ApiKey>
// ApiKey has no masked/preview field — only full `key`. The component masks via maskApiKey().
vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: vi.fn().mockResolvedValue({
      items: [
        {
          id: 1,
          key: 'sk-abcd1234efgh5678',
          name: 'test-key',
          status: 'active',
        },
      ],
      total: 1,
      page: 1,
      page_size: 1,
      pages: 1,
    }),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { api_base_url: 'https://demo.test/v1' },
  }),
}))

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

describe('UserDashboardQuickAccess', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the api_base_url', async () => {
    const w = mount(UserDashboardQuickAccess)
    await flushPromises()
    expect(w.text()).toContain('https://demo.test/v1')
  })

  it('renders the masked key (first 6 + last 4) when an active key exists, without leaking the full key', async () => {
    const w = mount(UserDashboardQuickAccess)
    await flushPromises()
    // maskApiKey('sk-abcd1234efgh5678') -> 'sk-abc...5678'
    expect(w.text()).toContain('sk-abc')
    expect(w.text()).toContain('5678')
    // Security: the full raw key must never appear in the DOM.
    expect(w.text()).not.toContain('sk-abcd1234efgh5678')
  })

  it('shows the create-key affordance when no active key is present', async () => {
    // Override the list mock for this case only.
    const { keysAPI } = await import('@/api/keys')
    ;(keysAPI.list as any).mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 1, pages: 0 })
    const w = mount(UserDashboardQuickAccess)
    await flushPromises()
    expect(w.text()).toContain('dashboard.quickAccess.noKey')
    expect(w.text()).toContain('dashboard.quickAccess.createKey')
  })
})
