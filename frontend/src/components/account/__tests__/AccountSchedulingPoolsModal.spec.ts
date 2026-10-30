import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountSchedulingPoolsModal from '../AccountSchedulingPoolsModal.vue'

const { list, members, policy, assign, updatePolicy } = vi.hoisted(() => ({ list: vi.fn(), members: vi.fn(), policy: vi.fn(), assign: vi.fn(), updatePolicy: vi.fn() }))
vi.mock('@/api/admin/accountSchedulingPools', () => ({ accountSchedulingPoolsAPI: { list, members, policy, assign, updatePolicy } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => true }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))

const make = () => mount(AccountSchedulingPoolsModal, {
  props: { show: true, selectedAccountIds: [9, 11], proxies: [{ id: 2, name: 'Proxy 2' }] },
  global: { stubs: { BaseDialog: { template: '<div><slot/><slot name="footer"/></div>' } } }
})

describe('account scheduling pool management', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    list.mockResolvedValue([{ id: 7, name: 'Free', enabled: true }])
    members.mockResolvedValue([{ account_id: 9, pool_id: 7, name: 'GPT-Free-01', state: 'standby', reason: 'pool_disabled', exit_group: 'shared' }, { account_id: 11, pool_id: 7, name: 'GPT-Free-02', state: 'active', reason: 'ready', exit_group: 'shared' }])
    policy.mockResolvedValue({ enabled: false, max_active_accounts: 3, proxy_exit_groups: {} })
    assign.mockResolvedValue(undefined); updatePolicy.mockResolvedValue(undefined)
  })
  it('keeps the shared policy disabled and persists explicit exit mappings', async () => {
    const wrapper = make(); await flushPromises()
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click')
    expect((wrapper.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('#pool-active-limit').element as HTMLInputElement).value).toBe('3')
    await wrapper.get('#pool-exit-direct').setValue(' same-exit ')
    await wrapper.get('#pool-exit-2').setValue('same-exit')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(updatePolicy).toHaveBeenCalledWith({ enabled: false, max_active_accounts: 3, proxy_exit_groups: { direct: 'same-exit', '2': 'same-exit' } })
  })
  it('assigns selection and submits a complete stable queue reorder', async () => {
    const wrapper = make(); await flushPromises()
    await wrapper.get('#account-pool-list').setValue('7'); await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.pools.reasons.pool_disabled')
    const add = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.pools.addSelected')!
    await add.trigger('click'); await flushPromises()
    expect(assign).toHaveBeenLastCalledWith(7, [9, 11])
    await wrapper.findAll('[aria-label="admin.accounts.pools.moveDown"]')[0]!.trigger('click'); await flushPromises()
    expect(assign).toHaveBeenLastCalledWith(7, [11, 9])
  })
})
