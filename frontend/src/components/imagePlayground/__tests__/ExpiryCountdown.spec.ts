import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: ref('zh-CN'),
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'imagePlayground.retention.countdown') return `remaining ${params?.time}`
      if (key === 'imagePlayground.retention.expired') return 'expired'
      if (key === 'imagePlayground.retention.dayUnit') return 'd'
      if (key === 'imagePlayground.retention.expiresAt') return `expires ${params?.time}`
      return key
    },
  }),
}))

import ExpiryCountdown from '@/components/imagePlayground/ExpiryCountdown.vue'

describe('ExpiryCountdown', () => {
  it('shows a live-style duration and switches to expired at the deadline', async () => {
    const now = Date.UTC(2026, 7, 1, 0, 0, 0)
    const wrapper = mount(ExpiryCountdown, {
      props: {
        expiresAt: (now + 25 * 60 * 60 * 1000 + 61 * 1000) / 1000,
        now,
      },
      global: { stubs: { Icon: true } },
    })

    expect(wrapper.text()).toBe('remaining 1d 01:01:01')
    expect(wrapper.attributes('title')).toContain('expires')

    await wrapper.setProps({ now: now + 25 * 60 * 60 * 1000 + 61 * 1000 })
    expect(wrapper.text()).toBe('expired')
    expect(wrapper.classes()).toContain('bg-red-700/85')
  })
})
