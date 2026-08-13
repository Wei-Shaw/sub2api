import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}))

describe('PaymentMethodSelector', () => {
  it('wraps large custom method collections without letting labels widen the selector', () => {
    const methods = Array.from({ length: 12 }, (_, index) => ({
      type: `custom_${index}`,
      display_name: `CUSTOM_PAYMENT_METHOD_${index}`,
      fee_rate: 0,
      available: true,
    }))

    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'custom_0',
        methods,
      },
    })

    const grid = wrapper.get('[data-testid="payment-method-grid"]')
    expect(grid.classes()).toEqual(expect.arrayContaining(['grid', 'sm:grid-cols-3', 'lg:grid-cols-4']))
    expect(grid.classes()).not.toContain('sm:flex')

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(methods.length)
    expect(buttons.every(button => button.classes().includes('min-w-0'))).toBe(true)
    expect(buttons.every((button, index) => button.attributes('title') === methods[index].display_name)).toBe(true)
    expect(wrapper.findAll('[data-testid="payment-method-label"]').every(label => label.classes().includes('truncate'))).toBe(true)
  })

  it('shows the configured display name for custom EasyPay methods', () => {
    const wrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'ldc',
        methods: [{ type: 'ldc', display_name: 'LDC Pay', fee_rate: 0, available: true }],
      },
    })

    expect(wrapper.text()).toContain('LDC Pay')
    expect(wrapper.text()).not.toContain('ldc')
    expect(wrapper.text()).not.toContain('payment.methods.ldc')
  })

  /*
   * The selected state is now the accent for every method — the provider hue no
   * longer rides on the border — so the old `border-[#02A9F1]` assertion has
   * nothing left to distinguish. The underlying rule it guarded is unchanged
   * and still worth guarding: `isBuiltInAlipayMethod` is a whole-token check,
   * so a custom EasyPay method named `card_alipay` must not be dressed as
   * Alipay. That now shows up in the mark rather than the border, which is
   * where the assertion moved.
   */
  it('does not give the Alipay mark to custom methods that contain built-in names', () => {
    const customWrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'card_alipay',
        methods: [{ type: 'card_alipay', display_name: 'Card Pay', fee_rate: 0, available: true }],
      },
    })
    const builtInWrapper = mount(PaymentMethodSelector, {
      props: {
        selected: 'alipay',
        methods: [{ type: 'alipay', fee_rate: 0, available: true }],
      },
    })

    const customIcon = customWrapper.get('img').attributes('src')
    const builtInIcon = builtInWrapper.get('img').attributes('src')

    expect(customIcon).not.toBe(builtInIcon)
    expect(customWrapper.get('button').attributes('aria-checked')).toBe('true')
    expect(customWrapper.get('button').classes()).toContain('border-accent')
  })
})
