import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'
import type { AdminUser } from '@/types'

const { getUserBalanceHistory } = vi.hoisted(() => ({
  getUserBalanceHistory: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserBalanceHistory }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const user: AdminUser = {
  id: 9,
  username: 'history-user',
  email: 'history@example.com',
  role: 'user',
  balance: 0,
  concurrency: 4,
  extra_concurrency: 2,
  status: 'active',
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z',
  notes: ''
}

describe('UserBalanceHistoryModal extra concurrency history', () => {
  beforeEach(() => {
    getUserBalanceHistory.mockReset()
    getUserBalanceHistory.mockResolvedValue({
      items: [
        {
          id: 1,
          code: 'ADMIN-EXTRA',
          type: 'admin_extra_concurrency',
          value: 2,
          status: 'used',
          used_by: 9,
          used_at: '2026-07-10T00:00:00Z',
          created_at: '2026-07-10T00:00:00Z',
          group_id: null,
          validity_days: 0,
          notes: 'pilot allowance'
        }
      ],
      total: 1,
      page: 1,
      page_size: 15,
      pages: 1,
      total_recharged: 0
    })
  })

  it('renders an administrator extra concurrency adjustment with its own label', async () => {
    const wrapper = mount(UserBalanceHistoryModal, {
      props: { show: false, user },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Select: true,
          Icon: true
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('redeem.extraConcurrencyAddedAdmin')
    expect(wrapper.text()).toContain('redeem.adminAdjustment')
    expect(wrapper.text()).not.toContain('common.unknown')
  })
})
