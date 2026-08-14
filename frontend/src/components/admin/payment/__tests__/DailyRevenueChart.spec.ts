import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { seriesPalette } from '@/components/charts/chartTheme'
import { setTheme } from '@/composables/useTheme'
import type { DailyPaymentStats } from '@/types/payment'

import DailyRevenueChart from '../DailyRevenueChart.vue'

/*
 * These tests deliberately do not touch a canvas. They assert on the two things
 * that are pure data — the dataset config handed to chart.js, and whether the
 * options object is a live `computed` rather than one frozen at construction.
 */

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template:
      '<div><div class="chart-data">{{ JSON.stringify(data) }}</div>' +
      '<div class="chart-options">{{ JSON.stringify(options) }}</div></div>',
  },
}))

/**
 * A reactive stand-in for the real `t`. The point is the reactivity: a
 * `computed` that calls it re-evaluates when the locale ref changes, and a
 * frozen object literal does not.
 */
const { locale } = vi.hoisted(() => ({ locale: { value: 'en' } }))

vi.mock('vue-i18n', async () => {
  const { ref } = await import('vue')
  const localeRef = ref('en')
  Object.defineProperty(locale, 'value', {
    get: () => localeRef.value,
    set: (next: string) => {
      localeRef.value = next
    },
  })

  const dictionary: Record<string, Record<string, string>> = {
    en: {
      'payment.admin.dailyRevenue': 'Daily Revenue',
      'payment.admin.revenue': 'Revenue',
      'payment.admin.orderCount': 'Order Count',
      'payment.admin.noData': 'No data',
    },
    zh: {
      'payment.admin.dailyRevenue': '每日收入',
      'payment.admin.revenue': '收入',
      'payment.admin.orderCount': '订单数',
      'payment.admin.noData': '暂无数据',
    },
  }

  return {
    useI18n: () => ({
      t: (key: string) => dictionary[localeRef.value]?.[key] ?? key,
    }),
  }
})

const data: DailyPaymentStats[] = [
  { date: '2026-08-01', amount: { USD: 10, CNY: 70 }, count: 3 },
  { date: '2026-08-02', amount: { USD: 4, CNY: 28 }, count: 1 },
]

function mountChart() {
  return mount(DailyRevenueChart, {
    props: { data },
    global: { stubs: { LoadingSpinner: true } },
  })
}

const readData = (wrapper: ReturnType<typeof mountChart>) =>
  JSON.parse(wrapper.find('.chart-data').text())

const readOptions = (wrapper: ReturnType<typeof mountChart>) =>
  JSON.parse(wrapper.find('.chart-options').text())

afterEach(() => {
  locale.value = 'en'
  setTheme('light')
})

describe('DailyRevenueChart', () => {
  it('never smooths the series', () => {
    const datasets = readData(mountChart()).datasets

    expect(datasets).toHaveLength(3)
    for (const dataset of datasets) {
      // `applyChartDefaults()` pins `line.tension` to 0 because bezier
      // smoothing draws values that were never measured. Re-declaring any of
      // these per dataset would silently opt back out of that.
      expect(dataset).not.toHaveProperty('tension')
      expect(dataset).not.toHaveProperty('lineTension')
      expect(dataset).not.toHaveProperty('cubicInterpolationMode')
      expect(dataset).not.toHaveProperty('pointRadius')
    }
  })

  it('draws every series from the token palette, with no colour reused', () => {
    const datasets = readData(mountChart()).datasets
    const palette = seriesPalette(false)
    const borders = datasets.map((dataset: { borderColor: string }) => dataset.borderColor)

    expect(borders).toEqual([palette[0], palette[1], palette[2]])
    expect(new Set(borders).size).toBe(borders.length)
    expect(JSON.stringify(datasets)).not.toMatch(/rgba?\(/)
  })

  it('recolours the series when the theme flips', async () => {
    const wrapper = mountChart()
    expect(readData(wrapper).datasets[0].borderColor).toBe(seriesPalette(false)[0])

    setTheme('dark')
    await nextTick()

    expect(readData(wrapper).datasets[0].borderColor).toBe(seriesPalette(true)[0])
  })

  it('retranslates the axis titles when the locale changes', async () => {
    const wrapper = mountChart()
    const before = readOptions(wrapper)
    expect(before.scales.y.title.text).toBe('Revenue')
    expect(before.scales.y1.title.text).toBe('Order Count')

    // The options object used to be a plain literal, so `t()` was resolved once
    // at construction and the axis titles kept the mount-time language forever.
    locale.value = 'zh'
    await nextTick()

    const after = readOptions(wrapper)
    expect(after.scales.y.title.text).toBe('收入')
    expect(after.scales.y1.title.text).toBe('订单数')
  })
})
