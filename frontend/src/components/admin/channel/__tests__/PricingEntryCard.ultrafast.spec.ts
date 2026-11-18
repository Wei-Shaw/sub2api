import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'

vi.mock('@/api/admin/channels', () => ({
  default: { getModelDefaultPricing: vi.fn() },
}))

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function createEntry(): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    fast_multiplier: null,
    flex_multiplier: null,
    ultrafast_multiplier: null,
    reasoning_effort_multipliers: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    time_pricing: { timezone: 'Asia/Shanghai', weekdays_only: false, periods: [] },
  }
}

const selector = '[data-testid="ultrafast-multiplier-input"]'

describe('PricingEntryCard Ultra Fast multiplier', () => {
  it('is shown next to Fast/Flex for OpenAI channel pricing', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry(), platform: 'openai', enableTierMultipliers: true },
    })

    expect(wrapper.find(selector).exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.channels.form.ultrafastMultiplierHint')
  })

  it('is hidden for platforms without an Ultra Fast tier', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry(), platform: 'anthropic', enableTierMultipliers: true },
    })

    expect(wrapper.find(selector).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.channels.form.fastMultiplier')
  })

  it('is hidden where channel tier multipliers are disabled', () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: createEntry(), platform: 'openai' },
    })

    expect(wrapper.find(selector).exists()).toBe(false)
  })

  it('emits the typed value and clears it back to null', async () => {
    const entry = createEntry()
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry, platform: 'openai', enableTierMultipliers: true },
    })
    const input = wrapper.find(selector)

    await input.setValue('4')
    await input.setValue('')

    const updates = wrapper.emitted('update') as PricingFormEntry[][]
    expect(updates[0][0].ultrafast_multiplier).toBe('4')
    expect(updates[1][0].ultrafast_multiplier).toBeNull()
    expect(updates[0][0].fast_multiplier).toBeNull()
  })
})
