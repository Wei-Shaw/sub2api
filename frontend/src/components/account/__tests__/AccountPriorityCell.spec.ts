import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountPriorityCell from '../AccountPriorityCell.vue'
import type { Account } from '@/types'
import { update } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({ update: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const account = (overrides: Partial<Account> = {}) => ({
  id: 7, name: 'Claude 1', platform: 'anthropic', type: 'oauth', priority: 3,
  ...overrides,
}) as Account

const mountCell = (value = account()) => mount(AccountPriorityCell, { props: { account: value } })

beforeEach(() => {
  vi.useFakeTimers()
  vi.mocked(update).mockReset().mockImplementation(async (id, req) => account({ id, priority: req.priority }))
})
afterEach(() => {
  vi.useRealTimers()
})

describe('AccountPriorityCell', () => {
  it('batches rapid +/- clicks into a single priority-only update', async () => {
    const wrapper = mountCell()
    await wrapper.get('[data-testid="account-priority-increment"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-increment"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-decrement"]').trigger('click')
    expect(wrapper.get('[data-testid="account-priority-value"]').text()).toBe('4')
    expect(update).not.toHaveBeenCalled()

    await vi.runAllTimersAsync()
    await flushPromises()

    expect(update).toHaveBeenCalledTimes(1)
    expect(update).toHaveBeenCalledWith(7, { priority: 4 })
    expect(wrapper.emitted('updated')?.[0]?.[0]).toMatchObject({ id: 7, priority: 4 })
  })

  it('does not go below 1', async () => {
    const wrapper = mountCell(account({ priority: 1 }))
    const dec = wrapper.get('[data-testid="account-priority-decrement"]')
    expect(dec.attributes('disabled')).toBeDefined()
    // 到达下限时按钮仍应随悬停显隐，而不是常驻半透明
    expect(dec.classes()).toContain('opacity-0')
    expect(dec.classes().some(c => c.startsWith('disabled:opacity'))).toBe(false)
    await dec.trigger('click')
    await vi.runAllTimersAsync()
    expect(update).not.toHaveBeenCalled()
  })

  it('saves a typed value on Enter and ignores unchanged input', async () => {
    const wrapper = mountCell()
    await wrapper.get('[data-testid="account-priority-value"]').trigger('click')
    const input = wrapper.get('[data-testid="account-priority-input"]')
    await input.setValue('12')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, { priority: 12 })

    vi.mocked(update).mockClear()
    await wrapper.setProps({ account: account({ priority: 12 }) })
    await wrapper.get('[data-testid="account-priority-value"]').trigger('click')
    await wrapper.get('[data-testid="account-priority-input"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(update).not.toHaveBeenCalled()
  })

  it('Escape cancels typing without saving', async () => {
    const wrapper = mountCell()
    await wrapper.get('[data-testid="account-priority-value"]').trigger('click')
    const input = wrapper.get('[data-testid="account-priority-input"]')
    await input.setValue('50')
    await input.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(update).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="account-priority-value"]').text()).toBe('3')
  })

  it('reverts and emits an error when the update fails', async () => {
    vi.mocked(update).mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountCell()
    await wrapper.get('[data-testid="account-priority-increment"]').trigger('click')
    await vi.runAllTimersAsync()
    await flushPromises()
    expect(wrapper.get('[data-testid="account-priority-value"]').text()).toBe('3')
    expect(wrapper.emitted('error')).toHaveLength(1)
    expect(wrapper.emitted('updated')).toBeUndefined()
  })
})
