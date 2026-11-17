import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageTimeCell from './UsageTimeCell.vue'
import { fullUsageDate, toggleUsageDate, usageTime } from './timeDisplay'

afterEach(() => { fullUsageDate.value = false; localStorage.removeItem('fork-usage-full-date') })
describe('usage time display', () => {
  it('defaults to seconds and switches mounted cells together, preserving full time on hover', async () => {
    const value = new Date(2026, 10, 17, 9, 8, 7).toISOString()
    const wrapper = mount(UsageTimeCell, { props: { value } })
    expect(wrapper.text()).toBe('09:08:07')
    expect(wrapper.attributes('title')).toBe('2026-11-17 09:08:07')
    toggleUsageDate(); await wrapper.vm.$nextTick()
    expect(wrapper.text()).toBe('2026-11-17 09:08:07')
    expect(localStorage.getItem('fork-usage-full-date')).toBe('true')
    expect(usageTime('invalid')).toBe('—')
    wrapper.unmount()
  })
})
