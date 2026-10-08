import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import AccountTableFilters from '../AccountTableFilters.vue'
import Select from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import type { AdminGroup } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: { count: number }) => params ? `${key} (${params.count})` : key
    })
  }
})

const defaultFilters = () => ({
  platform: '', type: '', status: '', privacy_mode: '', group: '',
  search: 'existing search', lite: '1', sort_by: 'priority', sort_order: 'asc'
})

function mountFilters(filters: Record<string, unknown> = defaultFilters()) {
  return mount(AccountTableFilters, {
    attachTo: document.body,
    props: {
      searchQuery: 'existing search', filters,
      groups: [{ id: 42, name: 'A very long group name' } as AdminGroup]
    },
    global: { stubs: { Teleport: true } }
  })
}

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.useRealTimers()
  document.body.innerHTML = ''
})

describe('AccountTableFilters', () => {
  it('toggles an accessible secondary panel without emitting changes or clearing filters', async () => {
    const filters = { ...defaultFilters(), type: 'bedrock', privacy_mode: '__unset__', group: 'ungrouped' }
    const wrapper = mountFilters(filters)
    const toggle = wrapper.get('button[aria-controls]')
    const panel = wrapper.get(`[id="${toggle.attributes('aria-controls')}"]`)

    expect(toggle.attributes('type')).toBe('button')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(panel.isVisible()).toBe(false)
    expect(wrapper.findAllComponents(Select).filter(select => select.isVisible())).toHaveLength(2)
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(panel.isVisible()).toBe(true)
    expect(wrapper.findAllComponents(Select).every(select => select.isVisible())).toBe(true)
    await toggle.trigger('click')
    expect(panel.isVisible()).toBe(false)
    await toggle.trigger('click')
    expect(wrapper.props('filters')).toEqual(filters)
    expect(wrapper.emitted('update:filters')).toBeUndefined()
    expect(wrapper.emitted('change')).toBeUndefined()
    wrapper.unmount()
  })

  it('indicates active secondary filters while collapsed and reacts to external resets', async () => {
    const wrapper = mountFilters({ ...defaultFilters(), platform: 'openai', status: 'active' })
    const toggle = wrapper.get('button[aria-controls]')
    expect(toggle.find('span').exists()).toBe(false)
    for (const [filters, count] of [
      [{ type: 'oauth', privacy_mode: '', group: '' }, 1],
      [{ type: 'setup-token', privacy_mode: '__unset__', group: 'ungrouped' }, 3],
      [{ type: '', privacy_mode: 'training_off', group: '42' }, 2]
    ] as const) {
      await wrapper.setProps({ filters: { ...defaultFilters(), ...filters } })
      expect(toggle.get('span').text()).toBe(String(count))
      expect(toggle.attributes('aria-label')).toBe(`admin.accounts.moreFiltersActive (${count})`)
      expect(toggle.attributes('aria-expanded')).toBe('false')
    }
    await wrapper.setProps({ filters: { type: null, privacy_mode: undefined, group: '' } })
    expect(toggle.find('span').exists()).toBe(false)
    expect(toggle.attributes('aria-label')).toBe('admin.accounts.moreFilters')
    wrapper.unmount()
  })

  it('preserves every option and emits a merged filter snapshot plus change for every selection', async () => {
    const filters = { ...defaultFilters(), platform: 'openai', type: 'oauth', status: 'active', privacy_mode: '__unset__', group: '42' }
    const wrapper = mountFilters(filters)
    await wrapper.get('button[aria-controls]').trigger('click')
    const expectedOptions = [
      ['platform', ['', ...CONCRETE_PLATFORM_OPTIONS.map(option => option.value)]],
      ['status', ['', 'active', 'inactive', 'error', 'rate_limited', 'temp_unschedulable', 'unschedulable']],
      ['type', ['', 'oauth', 'setup-token', 'apikey', 'bedrock']],
      ['privacy_mode', ['', '__unset__', 'training_off', 'training_set_cf_blocked', 'training_set_failed']],
      ['group', ['', 'ungrouped', '42']]
    ] as const
    let changes = 0
    for (const [index, [key, values]] of expectedOptions.entries()) {
      const select = wrapper.findAllComponents(Select)[index]
      expect(select.props('options').map(option => option.value)).toEqual(values)
      expect(select.props('modelValue')).toBe(filters[key])
      expect(select.get('button').attributes('aria-label')).not.toBe('Select option')
      for (const [optionIndex, value] of values.entries()) {
        await select.get('button').trigger('click')
        await select.findAll('[role="option"]')[optionIndex].trigger('click')
        changes += 1
        expect(wrapper.emitted('update:filters')?.at(-1)).toEqual([{ ...filters, [key]: value }])
        expect(wrapper.emitted('change')).toHaveLength(changes)
        expect(wrapper.props('filters')).toEqual(filters)
      }
    }
    expect(wrapper.findAllComponents(Select)[4].props('options')[2].label).toBe('A very long group name')
    wrapper.unmount()
  })

  it('preserves immediate search updates and the debounced search change event', async () => {
    vi.useFakeTimers()
    const wrapper = mountFilters()
    const search = wrapper.getComponent(SearchInput)
    expect(search.props('modelValue')).toBe('existing search')
    expect(search.props('placeholder')).toBe('admin.accounts.searchAccounts')
    await search.get('input').setValue('new search')
    expect(wrapper.emitted('update:searchQuery')).toEqual([['new search']])
    expect(wrapper.emitted('change')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.emitted('change')).toEqual([[]])
    expect(wrapper.emitted('update:filters')).toBeUndefined()
    wrapper.unmount()
  })

  it('uses shrinkable, wrapping controls and unique panel IDs for multiple instances', () => {
    const wrapper = mountFilters()
    expect(wrapper.classes()).toContain('min-w-0')
    expect(wrapper.get('.grid').classes()).toEqual(expect.arrayContaining(['grid-cols-2', 'sm:flex-wrap']))
    for (const select of wrapper.findAllComponents(Select)) {
      expect(select.classes()).toEqual(expect.arrayContaining(['min-w-0', 'w-full']))
      expect(select.classes()).not.toContain('w-40')
    }
    const pair = mount(defineComponent({
      components: { AccountTableFilters },
      setup: () => ({ filters: defaultFilters() }),
      template: '<div><AccountTableFilters search-query="" :filters="filters" /><AccountTableFilters search-query="" :filters="filters" /></div>'
    }))
    const toggles = pair.findAll('button[aria-controls]')
    expect(toggles[0].attributes('aria-controls')).not.toBe(toggles[1].attributes('aria-controls'))
    wrapper.unmount()
    pair.unmount()
  })
})
