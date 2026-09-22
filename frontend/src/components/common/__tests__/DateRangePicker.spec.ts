import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('DateRangePicker', () => {
  it.each(['2026-09-09T14:12:00+09:00', '2026-09-08T22:12:00-07:00'])('accepts offset-bearing fixed ranges: %s', async (startDate) => {
    const endDate = '2026-09-10T14:12:00+09:00'
    const wrapper = mount(DateRangePicker, {
      props: { startDate, endDate, enableTime: true, preset: null },
      global: { stubs: { Icon: true } }
    })
    await wrapper.find('.date-picker-trigger').trigger('click')
    const start = new Date(startDate)
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe(
      formatLocalDate(start) + 'T' + String(start.getHours()).padStart(2, '0') + ':' + String(start.getMinutes()).padStart(2, '0')
    )
    expect(wrapper.find('.date-picker-apply').attributes('disabled')).toBeUndefined()
    await wrapper.find('.date-picker-apply').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([{ startDate, endDate, preset: null }])
    wrapper.unmount()
  })

  it('emits minute-aligned rolling ranges and preserves calendar presets', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-10T06:30:48Z'))
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-09-09T06:30:00Z', endDate: '2026-09-10T06:30:00Z', enableTime: true, preset: 'last24Hours' },
      global: { stubs: { Icon: true } }
    })
    try {
      expect(wrapper.text()).toContain('Last 24 Hours')
      await wrapper.find('.date-picker-trigger').trigger('click')
      expect(wrapper.findAll('input[type="datetime-local"]')).toHaveLength(2)
      await wrapper.find('.date-picker-apply').trigger('click')
      expect(wrapper.emitted('change')?.[0]).toEqual([{
        startDate: '2026-09-09T06:31:00.000Z', endDate: '2026-09-10T06:31:00.000Z', preset: 'last24Hours'
      }])
      await wrapper.find('.date-picker-trigger').trigger('click')
      await wrapper.findAll('.date-picker-preset').find((b) => b.text() === 'Today')!.trigger('click')
      await wrapper.find('.date-picker-apply').trigger('click')
      expect(wrapper.emitted('update:startDate')?.[1]?.[0]).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    } finally { wrapper.unmount(); vi.useRealTimers() }
  })

  it('keeps manual minute inputs fixed, even for exactly 24 hours', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-09-09', endDate: '2026-09-10', enableTime: true, preset: null },
      global: { stubs: { Icon: true } }
    })
    await wrapper.find('.date-picker-trigger').trigger('click')
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('2026-09-08T14:37')
    await inputs[1].setValue('2026-09-09T14:37')
    await wrapper.find('.date-picker-apply').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([{
      startDate: new Date('2026-09-08T14:37').toISOString(),
      endDate: new Date('2026-09-09T14:37').toISOString(), preset: null
    }])
    wrapper.unmount()
  })

  it('rejects equal, reversed and empty boundaries', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-09-09', endDate: '2026-09-10', enableTime: true, preset: null },
      global: { stubs: { Icon: true } }
    })
    await wrapper.find('.date-picker-trigger').trigger('click')
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('2026-09-09T14:37')
    for (const end of ['2026-09-09T14:37', '2026-09-09T14:36', '']) {
      await inputs[1].setValue(end)
      expect(wrapper.find('.date-picker-apply').attributes('disabled')).toBeDefined()
      await wrapper.find('.date-picker-apply').trigger('click')
      expect(wrapper.emitted('change')).toBeUndefined()
    }
    wrapper.unmount()
  })

  it('displays timestamp dates in the browser timezone and cancels drafts on reopen', async () => {
    const start = '2026-09-09T23:37:00Z'
    const wrapper = mount(DateRangePicker, {
      props: { startDate: start, endDate: '2026-09-10T23:37:00Z', enableTime: true, preset: null },
      global: { stubs: { Icon: true } }
    })
    await wrapper.find('.date-picker-trigger').trigger('click')
    const input = wrapper.find('input')
    const expected = new Date(start)
    expect((input.element as HTMLInputElement).value).toBe(
      formatLocalDate(expected) + 'T' + String(expected.getHours()).padStart(2, '0') + ':' + String(expected.getMinutes()).padStart(2, '0')
    )
    await input.setValue('2026-09-08T10:00')
    await wrapper.find('.date-picker-trigger').trigger('click')
    await wrapper.find('.date-picker-trigger').trigger('click')
    expect(wrapper.emitted('change')).toBeUndefined()
    expect((wrapper.find('input').element as HTMLInputElement).value).not.toBe('2026-09-08T10:00')
    wrapper.unmount()
  })

  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.date-picker-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-apply').trigger('click')

    const nowAfterClick = new Date()
    const yesterdayAfterClick = new Date(nowAfterClick.getTime() - 24 * 60 * 60 * 1000)
    const expectedStart = formatLocalDate(yesterdayAfterClick)
    const expectedEnd = formatLocalDate(nowAfterClick)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })
})
