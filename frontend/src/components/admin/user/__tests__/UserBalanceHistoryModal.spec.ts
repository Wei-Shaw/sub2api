import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'

const { getUserBalanceHistory } = vi.hoisted(() => ({
  getUserBalanceHistory: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserBalanceHistory }
  }
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

const historyItem = (id: number, type: string, value: number, code: string) => ({
  id,
  code,
  type,
  value,
  status: 'used',
  used_by: 7,
  used_at: '2026-09-05T10:00:00Z',
  created_at: '2026-09-05T10:00:00Z',
  group_id: null,
  validity_days: 0,
  notes: ''
})

describe('UserBalanceHistoryModal source rendering', () => {
  beforeEach(() => {
    getUserBalanceHistory.mockReset()
    getUserBalanceHistory.mockResolvedValue({
      items: [
        historyItem(1, 'promo_balance', 3.5, 'PROMO-WELCOME'),
        historyItem(1, 'lottery_balance', 1.25, 'LOTTERY-2026-09-05'),
        historyItem(2, 'invitation', 0, 'INVITE-2026')
      ],
      total: 3,
      page: 1,
      page_size: 15,
      total_pages: 1,
      total_recharged: 4.75
    })
  })

  it('labels promo, lottery, and invitation records without displaying invitation as a zero recharge', async () => {
    const wrapper = mount(UserBalanceHistoryModal, {
      props: {
        show: false,
        user: {
          id: 7,
          email: 'user@example.test',
          username: 'user',
          notes: '',
          balance: 4.75,
          created_at: '2026-09-05T09:00:00Z'
        } as never
      },
      global: {
        stubs: {
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /></div>'
          },
          Select: true,
          Icon: {
            props: ['name'],
            template: '<span :data-icon="name" />'
          }
        }
      }
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getUserBalanceHistory).toHaveBeenCalledWith(7, 1, 15, undefined)
    expect(wrapper.text()).toContain('redeem.balanceAddedPromo')
    expect(wrapper.text()).toContain('+$3.50')
    expect(wrapper.text()).toContain('redeem.balanceAddedLottery')
    expect(wrapper.text()).toContain('+$1.25')
    expect(wrapper.text()).toContain('redeem.invitationUsed')
    expect(wrapper.text()).toContain('redeem.invitationUsedValue')
    expect(wrapper.text()).not.toContain('+$0.00')
    expect(wrapper.find('[data-icon="gift"]').exists()).toBe(true)
    expect(wrapper.find('[data-icon="trophy"]').exists()).toBe(true)
    expect(wrapper.find('[data-icon="key"]').exists()).toBe(true)
  })
})
