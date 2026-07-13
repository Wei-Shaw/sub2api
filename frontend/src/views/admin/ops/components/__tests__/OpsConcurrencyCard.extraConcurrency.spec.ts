import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const mockGetConcurrencyStats = vi.fn()
const mockGetAccountAvailabilityStats = vi.fn()
const mockGetUserConcurrencyStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getConcurrencyStats: (...args: any[]) => mockGetConcurrencyStats(...args),
    getAccountAvailabilityStats: (...args: any[]) => mockGetAccountAvailabilityStats(...args),
    getUserConcurrencyStats: (...args: any[]) => mockGetUserConcurrencyStats(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (key === 'admin.ops.concurrency.queued' && params) return `Queue ${params.count}`
        if (key === 'admin.ops.concurrency.standard') return 'Standard'
        if (key === 'admin.ops.concurrency.extra') return 'Extra'
        return key
      },
    }),
  }
})

describe('OpsConcurrencyCard extra concurrency', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetConcurrencyStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
    mockGetAccountAvailabilityStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
    mockGetUserConcurrencyStats.mockResolvedValue({
      enabled: true,
      user: {
        77: {
          user_id: 77,
          user_email: 'ops-extra@example.com',
          username: 'ops-extra',
          standard_current_in_use: 2,
          standard_max_capacity: 4,
          extra_current_in_use: 1,
          extra_max_capacity: 3,
          current_in_use: 3,
          max_capacity: 7,
          load_percentage: 42.86,
          waiting_in_queue: 5,
        },
      },
    })
  })

  it('shows standard and extra usage only in the user view while keeping waiting combined', async () => {
    const wrapper = mount(OpsConcurrencyCard, { props: { refreshToken: 0 } })
    await flushPromises()

    expect(wrapper.find('[data-testid="ops-user-standard-concurrency"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ops-user-extra-concurrency"]').exists()).toBe(false)

    await wrapper.get('button[title="admin.ops.concurrency.switchToUser"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="ops-user-standard-concurrency"]').text()).toContain('Standard')
    expect(wrapper.get('[data-testid="ops-user-standard-concurrency"]').text()).toContain('2/4')
    expect(wrapper.get('[data-testid="ops-user-extra-concurrency"]').text()).toContain('Extra')
    expect(wrapper.get('[data-testid="ops-user-extra-concurrency"]').text()).toContain('1/3')
    expect(wrapper.text()).toContain('Queue 5')
  })
})
