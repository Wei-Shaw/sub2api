import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import {
  PROVIDER_SEPAY,
  SEPAY_BANK_TRANSFER,
  SEPAY_CARD,
  SEPAY_NAPAS,
  WEBHOOK_PATHS,
} from '@/components/payment/providerConfig'
import type { ProviderInstance } from '@/types/payment'

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.sepayWebhookHint': 'Configure the SePay IPN endpoint.',
  'admin.settings.payment.field_merchantId': 'Merchant ID',
  'admin.settings.payment.field_secretKey': 'Secret Key',
  'admin.settings.payment.field_env': 'Environment',
  'admin.settings.payment.field_currency': 'Payment currency',
  'admin.settings.payment.validationFieldRequired': 'Missing {field}',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      const message = messages[key] ?? key
      if (!params) return message
      return Object.entries(params).reduce(
        (value, [name, replacement]) => value.replaceAll('{' + name + '}', replacement),
        message,
      )
    },
  }),
}))

function providerFactory(overrides: Partial<ProviderInstance> = {}): ProviderInstance {
  return {
    id: 1,
    provider_key: PROVIDER_SEPAY,
    name: 'SePay',
    config: {},
    supported_types: [SEPAY_BANK_TRANSFER],
    enabled: true,
    payment_mode: 'redirect',
    limits: '',
    sort_order: 0,
    ...overrides,
  }
}

function mountDialog(options: { editing?: ProviderInstance | null } = {}) {
  return mount(PaymentProviderDialog, {
    props: {
      show: true,
      saving: false,
      editing: options.editing ?? null,
      allKeyOptions: [{ value: PROVIDER_SEPAY, label: 'SePay' }],
      enabledKeyOptions: [{ value: PROVIDER_SEPAY, label: 'SePay' }],
      allPaymentTypes: [
        { value: SEPAY_BANK_TRANSFER, label: 'Bank Transfer' },
        { value: SEPAY_NAPAS, label: 'Napas' },
        { value: SEPAY_CARD, label: 'Card' },
      ],
      redirectLabel: 'Redirect',
    },
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>',
        },
        Select: {
          props: ['modelValue', 'options', 'disabled'],
          template: '<div />',
        },
        ToggleSwitch: {
          template: '<div />',
        },
      },
    },
  })
}

type DialogVm = {
  reset: (key: string) => void
  loadProvider: (provider: ProviderInstance) => void
  config: Record<string, string>
  form: { supported_types: string[]; payment_mode: string; provider_key: string; name: string }
  handleSave: () => void
}

describe('PaymentProviderDialog', () => {
  it('shows the SePay webhook endpoint so the admin can paste it into the merchant portal', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.sepayWebhookHint'])
    expect(wrapper.text()).toContain(WEBHOOK_PATHS[PROVIDER_SEPAY])
  })

  it('defaults a new instance to redirect mode and every SePay method', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_SEPAY)
    await nextTick()

    // SePay reaches its checkout through a signed POST form, so redirect is the
    // only mode that actually completes a payment.
    expect(vm.form.payment_mode).toBe('redirect')
    expect(vm.form.supported_types).toEqual([SEPAY_BANK_TRANSFER, SEPAY_NAPAS, SEPAY_CARD])
  })

  it('applies the SePay config defaults', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_SEPAY)
    await nextTick()

    expect(vm.config.env).toBe('production')
    expect(vm.config.currency).toBe('VND')
  })

  it('blocks saving until the merchant credentials are filled in', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_SEPAY)
    await nextTick()
    vm.handleSave()
    await nextTick()

    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('emits the merchant credentials and derived callback URLs on save', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_SEPAY)
    await nextTick()
    Object.assign(vm.config, { merchantId: 'MERCHANT_TEST', secretKey: 'sk_test_123' })
    vm.form.name = 'SePay VN'
    await nextTick()

    vm.handleSave()
    await nextTick()

    const saved = wrapper.emitted('save')
    expect(saved).toHaveLength(1)
    const payload = saved![0][0] as { provider_key: string; config: Record<string, string> }
    expect(payload.provider_key).toBe(PROVIDER_SEPAY)
    expect(payload.config.merchantId).toBe('MERCHANT_TEST')
    expect(payload.config.secretKey).toBe('sk_test_123')
    expect(payload.config.notifyUrl).toContain(WEBHOOK_PATHS[PROVIDER_SEPAY])
  })

  it('leaves the secret blank when editing so an untouched field preserves the stored value', async () => {
    // The admin GET API omits sensitive fields entirely; submitting a blank
    // secret is how the backend is told to keep the existing one.
    const stored = providerFactory({
      config: { merchantId: 'MERCHANT_TEST', env: 'sandbox', currency: 'VND' },
    })
    const wrapper = mountDialog({ editing: stored })
    const vm = wrapper.vm as unknown as DialogVm

    vm.loadProvider(stored)
    await nextTick()

    expect(vm.config.merchantId).toBe('MERCHANT_TEST')
    expect(vm.config.secretKey ?? '').toBe('')

    vm.handleSave()
    await nextTick()

    expect(wrapper.emitted('save')).toHaveLength(1)
  })
})
