import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import RedeemView from '../RedeemView.vue'

const { getHistory, redeem, getPublicSettings, refreshUser } = vi.hoisted(() => ({
  getHistory: vi.fn(),
  redeem: vi.fn(),
  getPublicSettings: vi.fn(),
  refreshUser: vi.fn()
}))

vi.mock('@/api', () => ({
  redeemAPI: { getHistory, redeem },
  authAPI: { getPublicSettings }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      id: 1,
      balance: 10,
      concurrency: 4,
      extra_concurrency: 2
    },
    refreshUser
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    fetchActiveSubscriptions: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        if (key === 'redeem.standardConcurrency') return 'Standard Concurrency'
        if (key === 'redeem.extraConcurrency') return 'Extra Concurrency'
        if (key === 'redeem.extraConcurrencyAddedAdmin') return 'Extra concurrency added by admin'
        if (key === 'redeem.adminAdjustment') return 'Admin adjustment'
        return key
      }
    })
  }
})

describe('RedeemView concurrency allowances', () => {
  beforeEach(() => {
    getHistory.mockReset()
    redeem.mockReset()
    getPublicSettings.mockReset()
    refreshUser.mockReset()
    getHistory.mockResolvedValue([
      {
        id: 2,
        code: 'ADMIN-EXTRA',
        type: 'admin_extra_concurrency',
        value: 2,
        status: 'used',
        used_at: '2026-07-10T00:00:00Z',
        created_at: '2026-07-10T00:00:00Z',
        notes: 'pilot allowance'
      }
    ])
    getPublicSettings.mockResolvedValue({ contact_info: '' })
  })

  it('shows standard and extra limits while rendering extra adjustments as admin-only history', async () => {
    const wrapper = mount(RedeemView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('Standard Concurrency: 4')
    expect(wrapper.text()).toContain('Extra Concurrency: 2')
    expect(wrapper.text()).toContain('Extra concurrency added by admin')
    expect(wrapper.text()).toContain('Admin adjustment')
    expect(wrapper.text()).not.toContain('common.unknown')
    expect(redeem).not.toHaveBeenCalled()
  })
})
