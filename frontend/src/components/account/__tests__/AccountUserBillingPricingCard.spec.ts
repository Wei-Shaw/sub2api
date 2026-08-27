import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import AccountUserBillingPricingCard from '../AccountUserBillingPricingCard.vue'

const ToggleStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ['update:modelValue'],
  template: '<button data-testid="toggle" @click="$emit(\'update:modelValue\', !modelValue)" />',
})

const PricingEntryCardStub = defineComponent({
  props: { entry: Object, platform: String },
  emits: ['update', 'remove'],
  template: '<div data-testid="pricing-entry">{{ platform }}</div>',
})

describe('AccountUserBillingPricingCard', () => {
  it('requires an explicit multiplier and supports adding exact model price entries', async () => {
    const wrapper = mount(AccountUserBillingPricingCard, {
      props: {
        rateEnabled: false,
        rateMultiplier: null,
        modelPricing: [],
      },
      global: {
        stubs: {
          Toggle: ToggleStub,
          Icon: true,
          PricingEntryCard: PricingEntryCardStub,
        },
      },
    })

    expect(wrapper.find('[data-testid="account-user-billing-rate-multiplier"]').exists()).toBe(false)
    await wrapper.get('[data-testid="account-user-billing-rate-toggle"]').trigger('click')
    expect(wrapper.emitted('update:rateEnabled')?.[0]).toEqual([true])

    const addButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.accounts.userBillingPricing.addModelPricing')
    )
    expect(addButton).toBeDefined()
    await addButton?.trigger('click')
    const nextPricing = wrapper.emitted('update:modelPricing')?.[0]?.[0] as any[]
    expect(nextPricing).toHaveLength(1)
    expect(nextPricing[0]).toMatchObject({ models: [], billing_mode: 'token' })
  })

  it('warns that complete price and account multiplier apply together', () => {
    const wrapper = mount(AccountUserBillingPricingCard, {
      props: {
        rateEnabled: true,
        rateMultiplier: 1.5,
        modelPricing: [{ models: ['gpt-5'], billing_mode: 'token', intervals: [] }] as any,
      },
      global: {
        stubs: {
          Toggle: ToggleStub,
          Icon: true,
          PricingEntryCard: PricingEntryCardStub,
        },
      },
    })

    expect(wrapper.get('[data-testid="account-user-billing-combined-warning"]').text()).toContain(
      'admin.accounts.userBillingPricing.combinedWarning'
    )
  })

  it('accepts ordinary decimal multiplier values in native number validation', () => {
    const wrapper = mount(AccountUserBillingPricingCard, {
      props: {
        rateEnabled: true,
        rateMultiplier: null,
        modelPricing: [],
      },
      global: {
        stubs: {
          Toggle: ToggleStub,
          Icon: true,
          PricingEntryCard: PricingEntryCardStub,
        },
      },
    })

    const input = wrapper.get<HTMLInputElement>('[data-testid="account-user-billing-rate-multiplier"]')
    input.element.value = '1.2'
    expect(input.element.validity.valid).toBe(true)
  })
})
