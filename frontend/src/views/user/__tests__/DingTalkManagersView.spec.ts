import { ref } from 'vue'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DingTalkManagersView from '../DingTalkManagersView.vue'
import Select from '@/components/common/Select.vue'

const state = vi.hoisted(() => ({ isAdmin: true, user: { id: 1 } }))
const api = vi.hoisted(() => ({ apps: vi.fn(), directory: vi.fn(), managerPage: vi.fn(), createManager: vi.fn(), patchManager: vi.fn(), increaseBudget: vi.fn(), managerGrants: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state }))
vi.mock('@/api/dingtalk', () => ({ dingTalkAPI: () => api, publicDingTalkApps: vi.fn().mockResolvedValue([]) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('en'), t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
enableAutoUnmount(afterEach)
const manager = { user_id: 2, name: 'Manager', limit_cents: 50000, used_cents: 10000, enabled: true, departments: [{ app_id: 'a', department_id: 3 }, { app_id: 'b', department_id: 7 }] }
function render() { return mount(DingTalkManagersView, { global: { stubs: { teleport: true } } }) }
function button(wrapper: ReturnType<typeof render>, label: string) { return wrapper.findAll('button').find(b => b.text() === label)! }

beforeEach(() => {
  vi.clearAllMocks(); state.isAdmin = true
  api.apps.mockResolvedValue([{ id: 'a', name: 'Engineering', enabled: true }, { id: 'b', name: 'Other app', enabled: true }])
  api.directory.mockResolvedValue({ synced_at: new Date().toISOString(), departments: [{ id: 2, parent_id: 1, name: 'Team' }, { id: 3, parent_id: 2, name: 'Child' }], members: [{ department_id: 3, staff_id: 'staff', name: 'Candidate', user_id: 3, balance: 20 }] })
  api.managerPage.mockResolvedValue({ items: [structuredClone(manager)], total: 1, page: 1, page_size: 20 })
  api.createManager.mockResolvedValue({}); api.patchManager.mockResolvedValue({}); api.increaseBudget.mockResolvedValue({})
  api.managerGrants.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
})

describe('Project manager menu', () => {
  it('requests managers in pages of 20', async () => {
    api.managerPage.mockResolvedValueOnce({ items: Array.from({ length: 20 }, (_, i) => ({ ...manager, user_id: i + 2 })), total: 21, page: 1, page_size: 20 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.get('[data-testid="manager-table"] tbody').findAll('tr')).toHaveLength(20)
    await button(wrapper, 'Next').trigger('click'); await flushPromises()
    expect(api.managerPage).toHaveBeenLastCalledWith(2)
    expect(wrapper.get('[data-testid="manager-table"] tbody').findAll('tr')).toHaveLength(1)
  })
  it('edits in a modal with a budget conflict check and leaves department permissions untouched', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    await button(wrapper, 'Edit').trigger('click'); await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('Edit manager')
    await wrapper.get('[data-testid="manager-limit"]').setValue(750)
    await wrapper.get('[role="dialog"] form').trigger('submit'); await flushPromises()
    expect(api.patchManager).toHaveBeenCalledWith(2, { enabled: true, limit_cents: 75000, expected_limit_cents: 50000 })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })
  it('adds a searchable candidate with a default budget of 500 and their own department', async () => {
    const wrapper = render(); await flushPromises()
    await button(wrapper, 'Add manager').trigger('click'); await flushPromises()
    expect((wrapper.get('[data-testid="manager-limit"]').element as HTMLInputElement).value).toBe('500')
    const select = wrapper.findComponent(Select)
    expect(select.props('searchable')).toBe(true)
    select.vm.$emit('update:modelValue', 3)
    await flushPromises()
    await wrapper.get('[role="dialog"] form').trigger('submit'); await flushPromises()
    expect(api.createManager).toHaveBeenCalledWith(expect.objectContaining({ user_id: 3, limit_cents: 50000, departments: [{ app_id: 'a', department_id: 3 }] }))
  })
  it('assigns permissions in a searchable collapsible modal while retaining other apps', async () => {
    const wrapper = render(); await flushPromises()
    await button(wrapper, 'Organization permissions').trigger('click'); await flushPromises()
    const tree = wrapper.get('[data-testid="manager-departments"]')
    expect(tree.text()).toContain('Child')
    expect(wrapper.get('[data-testid="selected-departments"]').text()).toContain('Team / Child')
    await wrapper.get('[data-testid="manager-department-search"]').setValue('Child')
    const checkbox = tree.findAll('label').find(l => l.text() === 'Child')!.get('input')
    expect((checkbox.element as HTMLInputElement).checked).toBe(true)
    await checkbox.setValue(false)
    await wrapper.get('[aria-label="Expand/collapse managed department Team"]').trigger('click')
    expect(tree.text()).not.toContain('Child')
    await wrapper.get('[role="dialog"] form').trigger('submit'); await flushPromises()
    expect(api.patchManager).toHaveBeenCalledWith(2, { departments: [{ app_id: 'b', department_id: 7 }] })
  })
  it('retries a budget increase with the same amount and request ID after a network failure', async () => {
    api.increaseBudget.mockRejectedValueOnce({ status: 0, message: 'Connection lost' }).mockResolvedValueOnce({})
    const wrapper = render(); await flushPromises()
    await button(wrapper, 'Increase allocation budget').trigger('click'); await flushPromises()
    await wrapper.get('[data-testid="budget-increase"]').setValue(25)
    await wrapper.get('[role="dialog"] form').trigger('submit'); await flushPromises()
    const args = api.increaseBudget.mock.calls[0]
    expect(args[0]).toBe(2); expect(args[1].amount).toBe(25)
    expect(wrapper.get('[data-testid="budget-increase"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[role="dialog"] form').trigger('submit'); await flushPromises()
    expect(api.increaseBudget.mock.calls[1]).toEqual(args)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })
  it('filters allocation history by manager and paginates beyond 100 records', async () => {
    api.managerGrants.mockResolvedValue({ items: [{ id: 1, actor_id: 2, target_id: 99, app_id: 'a', department_id: 3, amount_cents: 2500, created_at: new Date().toISOString() }], total: 121, page: 1, page_size: 20 })
    const wrapper = render(); await flushPromises()
    await button(wrapper, 'Allocation history').trigger('click'); await flushPromises()
    expect(api.managerGrants).toHaveBeenCalledWith(2, 1)
    expect(wrapper.get('[data-testid="manager-history"]').text()).toContain('#99')
    const next = wrapper.get('[data-testid="history-pagination"]').findAll('button').find(b => b.text() === 'Next')!
    await next.trigger('click'); await flushPromises()
    expect(api.managerGrants).toHaveBeenLastCalledWith(2, 2)
  })
  it('does not load administrator data for a regular user', async () => {
    state.isAdmin = false
    const wrapper = render(); await flushPromises()
    expect(api.managerPage).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('Only administrators')
  })
  it('opens the assigned app and locates a nested selection after collapsing and searching', async () => {
    api.managerPage.mockResolvedValue({ items: [{ ...manager, departments: [{ app_id: 'b', department_id: 3 }] }], total: 1 })
    const wrapper = render(); await flushPromises()
    await button(wrapper, 'Organization permissions').trigger('click'); await flushPromises()
    expect(api.directory).toHaveBeenLastCalledWith('b')
    const tree = wrapper.get('[data-testid="manager-departments"]')
    expect(tree.text()).toContain('Child')
    await wrapper.get('[aria-label="Expand/collapse managed department Team"]').trigger('click')
    expect(tree.text()).not.toContain('Child')
    await wrapper.get('[data-testid="manager-department-search"]').setValue('missing')
    await wrapper.get('[aria-label="Locate Team / Child"]').trigger('click'); await flushPromises()
    expect(tree.text()).toContain('Child')
    expect((tree.findAll('label').find(l => l.text() === 'Child')!.get('input').element as HTMLInputElement).checked).toBe(true)
    expect(api.patchManager).not.toHaveBeenCalled()
  })

})
