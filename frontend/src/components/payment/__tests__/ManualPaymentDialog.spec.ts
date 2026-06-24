import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ManualPaymentDialog from '../ManualPaymentDialog.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('../currency', () => ({
  formatPaymentAmount: (value: number) => `¥${value.toFixed(2)}`,
}))

const item = {
  name: '29 元订阅池',
  price: 29,
}

function mountDialog() {
  return mount(ManualPaymentDialog, {
    props: {
      show: true,
      item,
      localeCode: 'zh-CN',
    },
    global: {
      stubs: {
        Teleport: true,
        Transition: false,
      },
    },
  })
}

describe('ManualPaymentDialog', () => {
  it('shows plan amount and defaults to WeChat QR code', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).toContain('29 元订阅池')
    expect(wrapper.text()).toContain('¥29.00')
    expect(wrapper.find('[data-testid="manual-payment-wxpay-qr"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="manual-payment-alipay-qr"]').exists()).toBe(false)
  })

  it('switches to Alipay QR code', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="manual-payment-tab-alipay"]').trigger('click')

    expect(wrapper.find('[data-testid="manual-payment-wxpay-qr"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="manual-payment-alipay-qr"]').exists()).toBe(true)
  })

  it('shows submitted state and emits redeem action', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="manual-payment-complete"]').trigger('click')

    expect(wrapper.text()).toContain('payment.manual.submittedTitle')
    await wrapper.get('[data-testid="manual-payment-redeem"]').trigger('click')

    expect(wrapper.emitted('redeem')).toHaveLength(1)
  })
})
