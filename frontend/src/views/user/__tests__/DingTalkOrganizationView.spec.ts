import { ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DingTalkOrganizationView from '../DingTalkOrganizationView.vue'

const state = vi.hoisted(() => ({ isAdmin: false, user: { id: 1 } }))
const api = vi.hoisted(() => ({ apps: vi.fn(), directory: vi.fn(), managers: vi.fn(), grants: vi.fn(), grant: vi.fn(), saveManager: vi.fn(), saveApps: vi.fn(), sync: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state }))
vi.mock('@/api/dingtalk', () => ({ dingTalkAPI: () => api, publicDingTalkApps: vi.fn().mockResolvedValue([{ id: 'a', name: 'Engineering' }]) }))
vi.mock('@/api/user', () => ({ startOAuthBinding: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('en') }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

function button(wrapper: ReturnType<typeof mount>, label: string) { return wrapper.findAll('button').find(b => b.text() === label)! }

describe('DingTalk organization quota', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.isAdmin = false
    api.apps.mockResolvedValue([{ id: 'a', name: 'Engineering', enabled: true }])
    api.directory.mockResolvedValue({ synced_at: new Date().toISOString(), departments: [{ id: 2, parent_id: 1, name: 'Team' }, { id: 3, parent_id: 2, name: 'Child' }], members: [{ department_id: 3, staff_id: 'staff', name: 'Member', user_id: 2, balance: 20 }] })
    api.managers.mockResolvedValue([{ user_id: 1, name: 'Manager', limit_cents: 10000, used_cents: 2500, enabled: true, departments: [{ app_id: 'a', department_id: 2 }, { app_id: 'b', department_id: 5 }] }])
    api.grants.mockResolvedValue([])
    api.grant.mockResolvedValue({ id: 1 })
  })
  it('shows scoped departments and remaining budget without administrator controls', async () => {
    const wrapper = mount(DingTalkOrganizationView)
    await flushPromises()
    expect(wrapper.text()).toContain('$100.00 / $25.00 / $75.00')
    expect(wrapper.text()).not.toContain('Save applications')
    expect(wrapper.text()).not.toContain('Add manager')
    expect(wrapper.text()).not.toContain('Sync DingTalk directory')
    await button(wrapper, 'Child').trigger('click')
    expect(wrapper.text()).toContain('Member')
  })
  it('retries a failed allocation with the same request ID and amount', async () => {
    api.grant.mockRejectedValueOnce({ status: 0, message: 'Connection lost' }).mockResolvedValueOnce({ id: 1 })
    const wrapper = mount(DingTalkOrganizationView)
    await flushPromises()
    await button(wrapper, 'Child').trigger('click')
    await button(wrapper, 'Add quota').trigger('click')
    await wrapper.get('[data-testid="grant-amount"]').setValue(12.5)
    await wrapper.get('[role="region"] form').trigger('submit')
    await flushPromises()
    expect(api.grant).toHaveBeenCalledTimes(1)
    const input = api.grant.mock.calls[0][0]
    expect(input).toMatchObject({ app_id: 'a', department_id: 3, target_id: 2, amount: 12.5 })
    expect(input.request_id.length).toBeGreaterThan(15)
    expect(wrapper.get('[data-testid="grant-amount"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[role="region"] form').trigger('submit')
    await flushPromises()
    expect(api.grant.mock.calls[1][0]).toEqual(input)
    expect(wrapper.text()).toContain('Quota credited')
  })
  it('updates a manager budget while preserving assignments in other applications', async () => {
    state.isAdmin = true
    const wrapper = mount(DingTalkOrganizationView)
    await flushPromises()
    await button(wrapper, 'Edit').trigger('click')
    await wrapper.get('[data-testid="manager-limit"]').setValue(150)
    await button(wrapper, 'Save manager').element.closest('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.saveManager).toHaveBeenCalledWith(expect.objectContaining({ user_id: 1, limit_cents: 15000, departments: [{ app_id: 'a', department_id: 2 }, { app_id: 'b', department_id: 5 }] }))
  })
})
