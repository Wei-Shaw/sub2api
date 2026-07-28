import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProviderCard from '@/components/payment/ProviderCard.vue'
import { PAYMENT_MODE_REDIRECT } from '@/components/payment/providerConfig'
import type { ProviderInstance } from '@/types/payment'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

function providerFactory(overrides: Partial<ProviderInstance> = {}): ProviderInstance {
  return {
    id: 1,
    provider_key: 'alipay',
    name: 'Alipay',
    config: {},
    supported_types: ['alipay'],
    enabled: true,
    payment_mode: PAYMENT_MODE_REDIRECT,
    refund_enabled: false,
    allow_user_refund: false,
    limits: '',
    sort_order: 0,
    ...overrides,
  }
}

describe('ProviderCard', () => {
  it('renders the redirect payment mode label', () => {
    const wrapper = mount(ProviderCard, {
      props: {
        provider: providerFactory(),
        enabled: true,
        availableTypes: [],
      },
      global: {
        stubs: {
          Icon: true,
          ToggleSwitch: true,
        },
      },
    })

    expect(wrapper.text()).toContain('admin.settings.payment.modeRedirect')
  })
})
