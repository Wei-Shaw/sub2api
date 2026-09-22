import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('PlatformTypeBadge', () => {
  it('uses Kiro theme instead of Anthropic orange theme', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'kiro',
        type: 'oauth',
        planType: 'KIRO FREE'
      }
    })

    expect(wrapper.html()).toContain('bg-violet-100')
    expect(wrapper.html()).toContain('text-violet-700')
    expect(wrapper.html()).not.toContain('bg-orange-100')
  })

  it('shows an English Telemetry badge when enabled', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        planType: 'pro',
        telemetryEnabled: true
      }
    })

    expect(wrapper.get('[data-testid="platform-telemetry-badge"]').text()).toBe('Telemetry')
    expect(wrapper.text()).toContain('Pro 20x')
    expect(wrapper.text()).not.toMatch(/抗降智/)
  })

  it('hides the Telemetry badge when the switch is off', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        planType: 'plus'
      }
    })

    expect(wrapper.find('[data-testid="platform-telemetry-badge"]').exists()).toBe(false)
  })
})
