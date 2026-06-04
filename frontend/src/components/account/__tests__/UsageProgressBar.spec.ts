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

    expect(wrapper.text()).toContain('现在')
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
    expect(wrapper.text()).not.toContain('现在')
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
    expect(wrapper.text()).not.toContain('现在')
  })

  it('remaining-from-used 模式用已用百分比换算剩余量', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 87,
        displayMode: 'remaining-from-used',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('余13%')
    const percent = wrapper.find('span[title="剩余 13%，已用 87%"]')
    expect(percent.exists()).toBe(true)
    expect(percent.classes()).toContain('text-amber-600')
    expect(wrapper.find('.bg-amber-500').exists()).toBe(true)
  })

  it('remaining 模式直接显示传入的剩余百分比', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 83,
        displayMode: 'remaining',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('余83%')
    const percent = wrapper.find('span[title="剩余 83%，已用 17%"]')
    expect(percent.exists()).toBe(true)
    expect(percent.classes()).toContain('text-green-600')
    expect(wrapper.find('.bg-green-500').exists()).toBe(true)
  })
})
