import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

enableAutoUnmount(afterEach)

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

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
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
  afterEach(() => vi.useRealTimers())

  it.each([
    ['Today', 'today', '2026-03-01', '2026-03-01'],
    ['Yesterday', 'yesterday', '2026-02-28', '2026-02-28'],
    ['Last 24 Hours', 'last24Hours', '2026-02-28', '2026-03-01'],
    ['Last 7 Days', '7days', '2026-02-23', '2026-03-01'],
    ['Last 14 Days', '14days', '2026-02-16', '2026-03-01'],
    ['Last 30 Days', '30days', '2026-01-31', '2026-03-01'],
    ['This Month', 'thisMonth', '2026-03-01', '2026-03-01'],
    ['Last Month', 'lastMonth', '2026-02-01', '2026-02-28']
  ])('preserves the %s preset calculation and emitted identifier', async (label, preset, startDate, endDate) => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 2, 1, 12))
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-03-01', endDate: '2026-03-01' },
      global: { stubs: { Icon: true } }
    })
    await wrapper.get('.date-picker-trigger').trigger('click')
    const button = wrapper.findAll('.date-picker-preset').find((node) => node.text() === label)
    expect(button).toBeDefined()
    await button!.trigger('click')
    await wrapper.get('.date-picker-apply').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([{ startDate, endDate, preset }])
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
