import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GrokFreeIcon from '../GrokFreeIcon.vue'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('PlatformTypeBadge Grok plans', () => {
  it('renders FREE and BASIC as Grok Free with a lightweight plan icon', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'grok',
        type: 'oauth',
        planType: 'BASIC',
        subscriptionExpiresAt: '2027-01-01T00:00:00Z',
      },
    })

    expect(wrapper.text()).toContain('Grok Free')
    expect(wrapper.findComponent(GrokFreeIcon).exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-free-plan-icon"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-plan-icon"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('2027-01-01')

    await wrapper.setProps({ planType: 'FREE' })
    expect(wrapper.text()).toContain('Grok Free')
    expect(wrapper.findComponent(GrokFreeIcon).exists()).toBe(true)
  })

  it('keeps SuperGrok labels compatible and marks paid Grok plans', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'grok',
        type: 'oauth',
        planType: 'SuperGrok Heavy',
      },
    })

    expect(wrapper.text()).toContain('SuperGrok Heavy')
    expect(wrapper.find('[data-testid="grok-plan-icon"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="grok-free-plan-icon"]').exists()).toBe(false)

    await wrapper.setProps({ platform: 'openai', planType: 'free' })
    expect(wrapper.text()).toContain('Free')
    expect(wrapper.text()).not.toContain('Grok Free')
    expect(wrapper.find('[data-testid="grok-plan-icon"]').exists()).toBe(false)
  })

  /*
   * This replaces a test that pinned the plan chip to `bg-gray-100` /
   * `bg-cyan-100` / `bg-purple-100`. That contract said a plan is told apart
   * by its hue, which is exactly what the design system removed: a plan is a
   * label, and the label is already on screen. What still has to hold is that
   * plans are distinguishable without colour, and that the one plan value
   * which IS a status still reads as one.
   */
  it('tells plans apart by label and mark, not by hue', async () => {
    const tiers = [
      { planType: 'free', label: 'Grok Free', paidMark: false },
      { planType: 'supergrok', label: 'SuperGrok', paidMark: true },
      { planType: 'Heavy', label: 'Heavy', paidMark: true },
      { planType: 'supergrok_lite', label: 'SuperGrok Lite', paidMark: false },
    ]

    for (const tier of tiers) {
      const wrapper = mount(PlatformTypeBadge, {
        props: { platform: 'grok', type: 'oauth', planType: tier.planType },
      })
      expect(wrapper.text()).toContain(tier.label)
      expect(wrapper.find('[data-testid="grok-plan-icon"]').exists()).toBe(tier.paidMark)
      // No plan is worth a tint. Only `abnormal` is, and it is not a plan tier.
      expect(wrapper.html()).not.toContain('bg-danger-tint')
    }
  })

  it('marks an abnormal subscription as a problem, not as another tier', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'openai', type: 'oauth', planType: 'abnormal' },
    })

    expect(wrapper.html()).toContain('bg-danger-tint')
    expect(wrapper.html()).toContain('text-danger')
    // Colour is never the only channel: the chip still carries its label.
    expect(wrapper.text()).toContain('admin.accounts.subscriptionAbnormal')
  })

  it('uses a dedicated 12px currentColor Grok mark with a Free sparkle', () => {
    const wrapper = mount(GrokFreeIcon)

    expect(wrapper.element.tagName.toLowerCase()).toBe('svg')
    expect(wrapper.attributes('fill')).toBe('currentColor')
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['h-3', 'w-3']))
    expect(wrapper.findAll('path')).toHaveLength(2)
  })
})

describe('PlatformTypeBadge OpenAI authentication modes', () => {
  it('distinguishes Agent Identity, PAT, and OAuth accounts', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        authMode: 'agentIdentity',
      },
    })

    expect(wrapper.text()).toContain('Agent Identity')

    await wrapper.setProps({ authMode: 'personalAccessToken' })
    expect(wrapper.text()).toContain('PAT')
    expect(wrapper.text()).not.toContain('Agent Identity')

    await wrapper.setProps({ authMode: undefined })
    expect(wrapper.text()).toContain('OAuth')
  })
})
