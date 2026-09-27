import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import DashboardView from '../DashboardView.vue'

const { apiGet, refreshUser, showError, authState } = vi.hoisted(() => ({
  apiGet: vi.fn(),
  refreshUser: vi.fn(),
  showError: vi.fn(),
  authState: {
    user: { id: 1, username: 'alice', email: 'alice@example.com', balance: 12.5 } as Record<string, unknown>,
    isSimpleMode: false,
  },
}))

// Mock at the HTTP client so request count, params and timing are observed
// the same way regardless of which usage API helper the view calls.
vi.mock('@/api/client', () => ({ apiClient: { get: apiGet } }))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ ...authState, refreshUser }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ siteName: 'Sub2API', siteLogo: '', showError, cachedPublicSettings: {} }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, cachedPublicSettings: {} }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const LATENCY_MS = 100
const later = <T>(value: T) => new Promise<T>((resolve) => setTimeout(() => resolve(value), LATENCY_MS))

const usageLog = (id: number) => ({
  id, user_id: 1, api_key_id: 3, account_id: null, request_id: `req_${id}`, model: 'claude-sonnet-4-5',
  group_id: 2, subscription_id: null, input_tokens: 1200, output_tokens: 340, cache_creation_tokens: 0,
  cache_read_tokens: 800, cache_creation_5m_tokens: 0, cache_creation_1h_tokens: 0, input_cost: 0.0036,
  output_cost: 0.0051, cache_creation_cost: 0, cache_read_cost: 0.00024, total_cost: 0.00894,
  actual_cost: 0.00894, rate_multiplier: 1, long_context_billing_applied: false, billing_type: 0,
  stream: true, native_compaction_v2: false, duration_ms: 2310, first_token_ms: 640, image_count: 0,
  image_size: null, image_input_size: null, image_output_size: null, image_size_source: null,
  image_size_breakdown: null, image_input_tokens: 0, image_input_cost: 0, image_output_tokens: 0,
  image_output_cost: 0, user_agent: 'claude-cli/2.0.0', cache_ttl_overridden: false,
  created_at: '2026-09-27T08:00:00Z',
})

const stats = { total_api_keys: 1, active_api_keys: 1, total_requests: 10, today_requests: 2 }

function respond(url: string, config?: { params?: { page_size?: number } }) {
  switch (url) {
    case '/usage/dashboard/stats':
      return later({ data: stats })
    case '/usage/dashboard/trend':
      return later({ data: { trend: [] } })
    case '/usage/dashboard/models':
      return later({ data: { models: [] } })
    case '/usage': {
      const size = config?.params?.page_size ?? 20
      return later({ data: { items: Array.from({ length: size }, (_, i) => usageLog(i + 1)), total: 500 } })
    }
    case '/user/platform-quotas':
      return later({ data: { platform_quotas: [] } })
    default:
      return Promise.reject(new Error(`unexpected GET ${url}`))
  }
}

const mountView = () => mount(DashboardView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      LoadingSpinner: { template: '<div data-test="spinner" />' },
      UserDashboardStats: { template: '<div data-test="stats" />' },
      UserDashboardCharts: true,
      UserDashboardRecentUsage: { props: ['data', 'loading'], template: '<div data-test="recent">{{ data.length }}</div>' },
      UserDashboardQuickActions: { template: '<div data-test="quick-actions" />' },
      MeterValue: true,
      Icon: true,
      RouterLink: RouterLinkStub,
    },
  },
})

describe('user DashboardView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    apiGet.mockReset().mockImplementation(respond)
    refreshUser.mockReset().mockImplementation(() => later(authState.user))
    showError.mockReset()
    authState.user = { id: 1, username: 'alice', email: 'alice@example.com', balance: 12.5 }
    authState.isSimpleMode = false
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads stats in one round trip and fetches only the recent rows it shows', async () => {
    const wrapper = mountView()
    let elapsed = 0
    while (!wrapper.find('[data-test="stats"]').exists() && elapsed < 10 * LATENCY_MS) {
      await vi.advanceTimersByTimeAsync(10)
      elapsed += 10
    }

    // refreshUser and the stats request run concurrently: one latency, not two.
    expect(elapsed).toBe(LATENCY_MS)
    expect(wrapper.find('[data-test="spinner"]').exists()).toBe(false)

    const usageCalls = apiGet.mock.calls.filter(([url]) => url === '/usage')
    expect(usageCalls).toHaveLength(1)
    expect(usageCalls[0][1].params).toMatchObject({ page: 1, page_size: 5 })
    await flushPromises()
    expect(wrapper.get('[data-test="recent"]').text()).toBe('5')
    // 5 HTTP requests through the client plus the /auth/me refresh in the store.
    expect(apiGet).toHaveBeenCalledTimes(5)
    expect(refreshUser).toHaveBeenCalledTimes(1)
  })
})
