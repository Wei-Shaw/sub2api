import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'

const mocks = vi.hoisted(() => ({ getUserBalanceHistory: vi.fn() }))
const { getUserBalanceHistory } = mocks

vi.mock('@/api/admin', () => ({ adminAPI: { users: mocks } }))
vi.mock('@/utils/format', () => ({ formatDateTime: () => 'date' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  getUserBalanceHistory.mockReset()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => vi.restoreAllMocks())

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
  it('labels promo, lottery, and invitation records without displaying invitation as a zero recharge', async () => {
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

function result(id: number) {
  return { items: [{ id, type: 'admin_balance', value: id, notes: `History ${id}` }], total: 1, total_recharged: id }
}

function deferred() {
  let resolve!: (value: ReturnType<typeof result>) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<ReturnType<typeof result>>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

async function openDialog() {
  const wrapper = mount(UserBalanceHistoryModal, {
    props: { show: false, user: { id: 1, email: 'one@example.com', balance: 1 } as AdminUser },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        Icon: true,
        Select: true
      }
    }
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('UserBalanceHistoryModal request ordering', () => {
  it('keeps the new user history when an old response finishes later', async () => {
    const old = deferred()
    getUserBalanceHistory.mockReturnValueOnce(old.promise).mockResolvedValueOnce(result(20))
    const wrapper = await openDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, user: { id: 2, email: 'two@example.com', balance: 2 } as AdminUser })
    await flushPromises()
    old.resolve(result(10))
    await flushPromises()
    expect(wrapper.text()).toContain('History 20')
    expect(wrapper.text()).not.toContain('History 10')
    expect(wrapper.text()).toContain('$20.00')
  })

  it('does not end the current filter loading state when an old request finishes', async () => {
    const old = deferred()
    const current = deferred()
    getUserBalanceHistory.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await openDialog()
    wrapper.findComponent({ name: 'Select' }).vm.$emit('change', 'balance')
    await flushPromises()
    old.resolve(result(10))
    await flushPromises()
    expect(wrapper.find('svg.animate-spin').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('History 10')
    current.resolve(result(20))
    await flushPromises()
    expect(wrapper.find('svg.animate-spin').exists()).toBe(false)
    expect(wrapper.text()).toContain('History 20')
  })

  it.each(['close', 'unmount'])('ignores failures after %s', async (action) => {
    const pending = deferred()
    getUserBalanceHistory.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    if (action === 'close') await wrapper.setProps({ show: false })
    else wrapper.unmount()
    pending.reject(new Error('Stale error'))
    await flushPromises()
    expect(console.error).not.toHaveBeenCalled()
  })

  it('still reports current failures and ends loading', async () => {
    const error = new Error('Current error')
    getUserBalanceHistory.mockRejectedValueOnce(error)
    const wrapper = await openDialog()
    await flushPromises()
    expect(console.error).toHaveBeenCalledWith('Failed to load balance history:', error)
    expect(wrapper.find('svg.animate-spin').exists()).toBe(false)
  })
})
