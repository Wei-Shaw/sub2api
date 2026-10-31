import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser } from '@/types'
import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'

const mocks = vi.hoisted(() => ({ getUserBalanceHistory: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: mocks } }))
vi.mock('@/utils/format', () => ({ formatDateTime: () => 'date' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.clearAllMocks(); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => vi.restoreAllMocks())
function result(id: number) {
  return { items: [{ id, type: 'admin_balance', value: id, notes: `History ${id}` }], total: 1, total_recharged: id }
}
function deferred() {
  let resolve!: (value: ReturnType<typeof result>) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<ReturnType<typeof result>>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
async function openDialog() {
  const wrapper = mount(UserBalanceHistoryModal, {
    props: { show: false, user: { id: 1, email: 'one@example.com', balance: 1 } as AdminUser },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' }, Icon: true, Select: true } }
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('UserBalanceHistoryModal request ordering', () => {
  it('keeps the new user history when an old response finishes later', async () => {
    const old = deferred()
    mocks.getUserBalanceHistory.mockReturnValueOnce(old.promise).mockResolvedValueOnce(result(20))
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
    mocks.getUserBalanceHistory.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
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
    mocks.getUserBalanceHistory.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    if (action === 'close') await wrapper.setProps({ show: false })
    else wrapper.unmount()
    pending.reject(new Error('Stale error'))
    await flushPromises()
    expect(console.error).not.toHaveBeenCalled()
  })

  it('still reports current failures and ends loading', async () => {
    const error = new Error('Current error')
    mocks.getUserBalanceHistory.mockRejectedValueOnce(error)
    const wrapper = await openDialog()
    await flushPromises()
    expect(console.error).toHaveBeenCalledWith('Failed to load balance history:', error)
    expect(wrapper.find('svg.animate-spin').exists()).toBe(false)
  })
})

describe('UserBalanceHistoryModal bot-sales credits', () => {
  it('shows full order and linked payment with a dedicated filter', async () => {
    mocks.getUserBalanceHistory.mockResolvedValue({
      items: [{ id: -1000000000326, code: 'BS_1793192965095', type: 'bot_sales_balance', value: 110,
        used_at: '2026-10-28T13:10:02Z', external_order_item_id: '2898', payment_order_id: 1075 }],
      total: 1, total_recharged: 110
    })
    const wrapper = await openDialog()
    await flushPromises()
    expect(wrapper.text()).toContain('redeem.balanceAddedBotSales')
    expect(wrapper.text()).toContain('BS_1793192965095')
    expect(wrapper.text()).toContain('2898')
    expect(wrapper.text()).toContain('1075')
    expect(wrapper.text()).toContain('+$110.00')
    wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', 'bot_sales_balance')
    wrapper.findComponent({ name: 'Select' }).vm.$emit('change')
    await flushPromises()
    expect(mocks.getUserBalanceHistory).toHaveBeenCalledWith(1, 1, 15, 'bot_sales_balance')
  })

  it('shows old fulfillment without inventing a payment order', async () => {
    mocks.getUserBalanceHistory.mockResolvedValueOnce({
      items: [{ id: -1000000000001, code: 'BS_LEGACY', type: 'bot_sales_balance', value: 10,
        used_at: '2026-08-22T10:00:00Z', external_order_item_id: 'old-item' }],
      total: 1, total_recharged: 10
    })
    const wrapper = await openDialog()
    await flushPromises()
    expect(wrapper.text()).toContain('BS_LEGACY')
    expect(wrapper.text()).toContain('old-item')
    expect(wrapper.text()).not.toContain('admin.users.botSalesPaymentOrder')
  })
})
