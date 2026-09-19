import { ref } from 'vue'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import DingTalkStatisticsView from '../DingTalkStatisticsView.vue'
import Select from '@/components/common/Select.vue'

const api = vi.hoisted(() => ({ statistics: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false }) }))
vi.mock('@/api/dingtalk', () => ({ dingTalkAPI: () => api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('en'), t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
enableAutoUnmount(afterEach)

describe('Organization quota statistics', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.statistics.mockResolvedValue({
      organizations: [{ id: 'a', name: 'Company', company_id: 'corp', departments: [{ id: 2, name: 'Team', parent_id: 1 }] }],
      companies: [{ id: 'corp', name: 'Company', members: 2, requests: 3, cost: 5 }],
      departments: [{ id: '2', app_id: 'a', name: 'Team', members: 2, requests: 3, cost: 5 }],
      users: [{ id: '9', name: 'Member', requests: 3, cost: 5 }],
      total: { members: 2, requests: 3, cost: 5 }
    })
  })
  it('uses inclusive local dates and sends company and department filters', async () => {
    const wrapper = mount(DingTalkStatisticsView)
    await flushPromises()
    await wrapper.get('[data-testid="statistics-start"]').setValue('2026-09-01')
    await wrapper.get('[data-testid="statistics-end"]').setValue('2026-09-19')
    const selects = wrapper.findAllComponents(Select)
    selects[0]!.vm.$emit('update:modelValue', 'corp')
    selects[1]!.vm.$emit('update:modelValue', 'a:2')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.statistics).toHaveBeenLastCalledWith({ start: new Date('2026-09-01T00:00:00').toISOString(), end: new Date('2026-09-20T00:00:00').toISOString(), company_id: 'corp', app_id: 'a', department_id: 2 })
    expect(wrapper.text()).toContain('$5.0000')
    await wrapper.findAll('[role="tab"]')[2]!.trigger('click')
    expect(wrapper.get('tbody').text()).toContain('Member')
    await wrapper.get('input[aria-label="Search rankings"]').setValue('missing')
    expect(wrapper.text()).toContain('No matching data')
  })
  it('rejects reversed dates without requesting and hides stale results after API failure', async () => {
    const wrapper = mount(DingTalkStatisticsView)
    await flushPromises()
    await wrapper.get('[data-testid="statistics-start"]').setValue('2026-09-20')
    await wrapper.get('[data-testid="statistics-end"]').setValue('2026-09-01')
    await wrapper.get('form').trigger('submit')
    expect(api.statistics).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toContain('valid date range')
    await wrapper.get('[data-testid="statistics-end"]').setValue('2026-09-21')
    api.statistics.mockRejectedValue(new Error('Forbidden'))
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('Forbidden')
    expect(wrapper.find('table').exists()).toBe(false)
  })
  it('renders company and department rankings as expandable trees with sibling cost ordering', async () => {
    const data = await api.statistics()
    data.organizations[0].departments.push({ id: 3, name: 'Low cost', parent_id: 2 }, { id: 4, name: 'High cost', parent_id: 2 })
    data.departments.push(
      { id: '3', app_id: 'a', name: 'Low cost', members: 1, requests: 1, cost: 1 },
      { id: '4', app_id: 'a', name: 'High cost', members: 1, requests: 2, cost: 4 }
    )
    const wrapper = mount(DingTalkStatisticsView)
    await flushPromises()
    expect(wrapper.get('tbody').findAll('tr')).toHaveLength(2)
    expect(wrapper.get('[aria-label="Expand/collapse Team"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('[aria-label="Expand/collapse Team"]').trigger('click')
    const rows = wrapper.get('tbody').findAll('tr')
    expect(rows.map(row => row.attributes('data-depth'))).toEqual(['0', '1', '2', '2'])
    expect(rows[2]!.text()).toContain('High cost')
    expect(rows[3]!.text()).toContain('Low cost')
    await wrapper.get('[aria-label="Expand/collapse Company"]').trigger('click')
    expect(wrapper.get('tbody').findAll('tr')).toHaveLength(1)
    await wrapper.get('input[aria-label="Search rankings"]').setValue('Low cost')
    expect(wrapper.get('tbody').findAll('tr')).toHaveLength(3)
    expect(wrapper.get('tbody').text()).toContain('Company')
    expect(wrapper.get('tbody').text()).toContain('Team')
    expect(wrapper.get('tbody').text()).not.toContain('High cost')
    await wrapper.get('input[aria-label="Search rankings"]').setValue('')
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click')
    expect(wrapper.get('tbody').findAll('tr').map(row => row.attributes('data-depth'))).toEqual(['0', '1', '1'])
    await wrapper.get('[aria-label="Expand/collapse Team"]').trigger('click')
    expect(wrapper.get('tbody').findAll('tr')).toHaveLength(1)
  })

})
