import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import MonitorHero from './MonitorHero.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('MonitorHero', () => {
  it('uses blue for the operational summary chip and dot', () => {
    const wrapper = mount(MonitorHero, {
      props: {
        overallStatus: 'operational',
        intervalSeconds: 60,
        window: '7d',
        loading: false,
      },
      global: {
        stubs: {
          Icon: true,
          AutoRefreshButton: true,
        },
      },
    })

    expect(wrapper.get('[role="tablist"] + span').classes()).toContain('bg-blue-100')
    expect(wrapper.get('[role="tablist"] + span > span').classes()).toContain('bg-blue-500')
  })
})
