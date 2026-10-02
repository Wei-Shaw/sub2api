import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import {
  GPMPAY_BANK_TRANSFER,
  PROVIDER_GPMPAY,
  WEBHOOK_PATHS,
} from '@/components/payment/providerConfig'
import type { ProviderInstance } from '@/types/payment'

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.gpmPayWebhookHint': 'Configure the GPM Pay webhook endpoint.',
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
    provider_key: PROVIDER_GPMPAY,
    name: 'GPM Pay',
    config: {},
    supported_types: [GPMPAY_BANK_TRANSFER],
    enabled: true,
    payment_mode: '',
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
      allKeyOptions: [{ value: PROVIDER_GPMPAY, label: 'GPM Pay' }],
      enabledKeyOptions: [{ value: PROVIDER_GPMPAY, label: 'GPM Pay' }],
      allPaymentTypes: [
        { value: GPMPAY_BANK_TRANSFER, label: 'VietQR' },
      ],
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
  form: { supported_types: string[]; provider_key: string; name: string }
  handleSave: () => void
}

describe('PaymentProviderDialog', () => {
  it('shows the GPM Pay webhook endpoint so the admin can paste it into the gateway portal', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.gpmPayWebhookHint'])
    expect(wrapper.text()).toContain(WEBHOOK_PATHS[PROVIDER_GPMPAY])
  })

  it('defaults a new instance to the VietQR method', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_GPMPAY)
    await nextTick()

    expect(vm.form.supported_types).toEqual([GPMPAY_BANK_TRANSFER])
  })

  it('applies the GPM Pay config defaults', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_GPMPAY)
    await nextTick()

    expect(vm.config.allowSimulated).toBe('false')
  })

  it('blocks saving until the merchant credentials are filled in', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_GPMPAY)
    await nextTick()
    vm.handleSave()
    await nextTick()

    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('emits the merchant credentials and derived callback URLs on save', async () => {
    const wrapper = mountDialog()
    const vm = wrapper.vm as unknown as DialogVm

    vm.reset(PROVIDER_GPMPAY)
    await nextTick()
    Object.assign(vm.config, {
      apiToken: 'tok_test_123',
      webhookSecret: 'whsec_test',
      bankBin: '970422',
      accountNumber: '0123456789',
    })
    vm.form.name = 'GPM Pay VN'
    await nextTick()

    vm.handleSave()
    await nextTick()

    const saved = wrapper.emitted('save')
    expect(saved).toHaveLength(1)
    const payload = saved![0][0] as { provider_key: string; payment_mode: string; config: Record<string, string> }
    expect(payload.provider_key).toBe(PROVIDER_GPMPAY)
    expect(payload.payment_mode).toBe('')
    expect(payload.config.apiToken).toBe('tok_test_123')
    expect(payload.config.webhookSecret).toBe('whsec_test')
    expect(payload.config.notifyUrl).toContain(WEBHOOK_PATHS[PROVIDER_GPMPAY])
    // The payer never leaves our QR page, so there is no return URL.
    expect(payload.config.returnUrl).toBeUndefined()
  })

  it('leaves the secret blank when editing so an untouched field preserves the stored value', async () => {
    // The admin GET API omits sensitive fields entirely; submitting a blank
    // secret is how the backend is told to keep the existing one.
    const stored = providerFactory({
      config: { bankBin: '970422', accountNumber: '0123456789', allowSimulated: 'false' },
    })
    const wrapper = mountDialog({ editing: stored })
    const vm = wrapper.vm as unknown as DialogVm

    vm.loadProvider(stored)
    await nextTick()

    expect(vm.config.accountNumber).toBe('0123456789')
    expect(vm.config.apiToken ?? '').toBe('')
    expect(vm.config.webhookSecret ?? '').toBe('')

    vm.handleSave()
    await nextTick()

    expect(wrapper.emitted('save')).toHaveLength(1)
  })
})
