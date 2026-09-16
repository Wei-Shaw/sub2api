import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageProgressBar from '../UsageProgressBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('UsageProgressBar', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-17T00:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('showNowWhenIdle=true 且利用率为 0 时显示“现在”', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 0,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: true,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('2h 30m')
  })

  it('showNowWhenIdle=true 但利用率大于 0 时显示倒计时', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '7d',
        utilization: 12,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: true,
        color: 'emerald'
      }
    })

    expect(wrapper.text()).toContain('2h 30m')
    expect(wrapper.text()).not.toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('usage.resetPending')
  })

  it('showNowWhenIdle=false 时保持原有倒计时行为', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '1d',
        utilization: 0,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: false,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('2h 30m')
    expect(wrapper.text()).not.toContain('usage.resetNow')
  })

  it('resetsAt 已过期且利用率大于 0 时显示「待刷新」', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 53,
        // 早于 fake system time 2026-03-17T00:00:00Z
        resetsAt: '2026-03-16T22:00:00Z',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetPending')
    expect(wrapper.text()).not.toContain('usage.resetNow')
  })

  it('resetsAt 已过期且利用率为 0 时仍显示「现在」', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 0,
        resetsAt: '2026-03-16T22:00:00Z',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('usage.resetPending')
  })

  it('剩余容量模式在 100% 时显示满格绿色', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: 'Req',
        utilization: 100,
        remainingCapacity: true,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('100%')
    expect(wrapper.get('.h-1\\.5 > div').attributes('style')).toContain('width: 100%')
    expect(wrapper.get('.h-1\\.5 > div').classes()).toContain('bg-green-500')
  })

  it('剩余容量模式在低量和耗尽时缩短并变红', async () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: 'Req',
        utilization: 15,
        remainingCapacity: true,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('15%')
    expect(wrapper.get('.h-1\\.5 > div').attributes('style')).toContain('width: 15%')
    expect(wrapper.get('.h-1\\.5 > div').classes()).toContain('bg-red-500')

    await wrapper.setProps({ utilization: 0 })

    expect(wrapper.text()).toContain('0%')
    expect(wrapper.get('.h-1\\.5 > div').attributes('style')).toContain('width: 0%')
    expect(wrapper.get('.h-1\\.5 > div').classes()).toContain('bg-red-500')
  })

  it('默认利用率模式仍把超限显示为满格红色', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 120,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('120%')
    expect(wrapper.get('.h-1\\.5 > div').attributes('style')).toContain('width: 100%')
    expect(wrapper.get('.h-1\\.5 > div').classes()).toContain('bg-red-500')
  })

  it('默认利用率模式按 75/90 阈值提前预警分级', () => {
    const mountAt = (utilization: number) =>
      mount(UsageProgressBar, {
        props: { label: '5h', utilization, color: 'indigo' }
      })

    // 条形配色：74 绿 / 75 与 89 黄 / 90 红
    expect(mountAt(74).get('.h-1\\.5 > div').classes()).toContain('bg-green-500')
    expect(mountAt(75).get('.h-1\\.5 > div').classes()).toContain('bg-amber-500')
    expect(mountAt(89).get('.h-1\\.5 > div').classes()).toContain('bg-amber-500')
    expect(mountAt(90).get('.h-1\\.5 > div').classes()).toContain('bg-red-500')

    // 百分比文本同步分级
    expect(mountAt(74).get('.h-1\\.5 + span').classes()).toContain('text-gray-600')
    expect(mountAt(75).get('.h-1\\.5 + span').classes()).toContain('text-amber-600')
    expect(mountAt(89).get('.h-1\\.5 + span').classes()).toContain('text-amber-600')
    expect(mountAt(90).get('.h-1\\.5 + span').classes()).toContain('text-red-600')
  })

  it('labelWidth 默认 fixed：标签保持定宽居中，百分比列不变', () => {
    const wrapper = mount(UsageProgressBar, {
      props: { label: '5h', utilization: 30, color: 'indigo' }
    })

    const label = wrapper.get('.gap-1 > span')
    expect(label.classes()).toContain('w-[32px]')
    expect(label.classes()).toContain('text-center')
    expect(label.classes()).not.toContain('max-w-[72px]')

    const percent = wrapper.get('.h-1\\.5 + span')
    expect(percent.classes()).toContain('w-[32px]')
    expect(percent.classes()).toContain('text-right')
  })

  it('labelWidth=auto 时标签限宽截断左对齐，百分比列保持不变', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: 'Pro/7 天',
        utilization: 30,
        color: 'purple',
        labelWidth: 'auto'
      }
    })

    const label = wrapper.get('.gap-1 > span')
    expect(label.text()).toBe('Pro/7 天')
    expect(label.classes()).toContain('max-w-[72px]')
    expect(label.classes()).toContain('truncate')
    expect(label.classes()).toContain('text-left')
    expect(label.classes()).not.toContain('w-[32px]')
    expect(label.classes()).not.toContain('text-center')

    const percent = wrapper.get('.h-1\\.5 + span')
    expect(percent.classes()).toContain('w-[32px]')
    expect(percent.classes()).toContain('text-right')
  })
})
