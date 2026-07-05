import { describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import { nextTick REDACTED from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import { STRIPE_SDK_API_VERSION REDACTED from '@/components/payment/providerConfig'
import type { ProviderInstance REDACTED from '@/types/payment'

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.easypayCustomMethods': 'Custom EasyPay methods',
  'admin.settings.payment.easypayCustomMethodsHint': 'Add provider-specific EasyPay type values.',
  'admin.settings.payment.addCustomMethod': 'Add method',
  'admin.settings.payment.customMethodType': 'Payment type',
  'admin.settings.payment.customMethodUpstreamType': 'Upstream type',
  'admin.settings.payment.customMethodDisplayName': 'Display name',
  'admin.settings.payment.paymentGuideTrigger': 'View payment guide',
  'admin.settings.payment.alipayGuideSummary': 'Desktop prefers QR precreate and falls back to cashier; mobile prefers WAP checkout.',
  'admin.settings.payment.wxpayGuideSummary': 'Desktop prefers Native QR; mobile routes to JSAPI or H5 based on browser context.',
  'admin.settings.payment.airwallexGuideSummary': 'Use Payment Acceptance read/write only.',
  'admin.settings.payment.stripeWebhookHint': 'Configure Stripe webhook.',
  'admin.settings.payment.stripeWebhookApiVersionHint': 'Use Stripe API version {versionREDACTED.',
  'admin.settings.payment.airwallexWebhookHint': 'Select payment_intent.succeeded and use the latest stable API version.',
REDACTED

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      const message = messages[key] ?? key
      if (!params) return message
      return Object.entries(params).reduce(
        (value, [name, replacement]) => value.replaceAll(`{${nameREDACTEDREDACTED`, replacement),
        message,
      )
    REDACTED,
  REDACTED),
REDACTED))

function providerFactory(overrides: Partial<ProviderInstance> = {REDACTED): ProviderInstance {
  return {
    id: 1,
    provider_key: 'airwallex',
    name: 'Airwallex',
    config: {REDACTED,
    supported_types: ['airwallex'],
    enabled: true,
    payment_mode: '',
    refund_enabled: false,
    allow_user_refund: false,
    limits: '',
    sort_order: 0,
    ...overrides,
  REDACTED
REDACTED

function mountDialog(options: { editing?: ProviderInstance | null REDACTED = {REDACTED) {
  return mount(PaymentProviderDialog, {
    props: {
      show: true,
      saving: false,
      editing: options.editing ?? null,
      allKeyOptions: [
        { value: 'easypay', label: 'EasyPay' REDACTED,
        { value: 'alipay', label: 'Alipay' REDACTED,
        { value: 'wxpay', label: 'WeChat Pay' REDACTED,
        { value: 'stripe', label: 'Stripe' REDACTED,
        { value: 'airwallex', label: 'Airwallex' REDACTED,
      ],
      enabledKeyOptions: [
        { value: 'easypay', label: 'EasyPay' REDACTED,
        { value: 'alipay', label: 'Alipay' REDACTED,
        { value: 'wxpay', label: 'WeChat Pay' REDACTED,
        { value: 'airwallex', label: 'Airwallex' REDACTED,
      ],
      allPaymentTypes: [
        { value: 'alipay', label: 'Alipay' REDACTED,
        { value: 'wxpay', label: 'WeChat Pay' REDACTED,
      ],
      redirectLabel: 'Redirect',
    REDACTED,
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>',
        REDACTED,
        Select: {
          props: ['modelValue', 'options', 'disabled'],
          template: '<div />',
        REDACTED,
        ToggleSwitch: {
          template: '<div />',
        REDACTED,
      REDACTED,
    REDACTED,
  REDACTED)
REDACTED

describe('PaymentProviderDialog payment guide', () => {
  it('shows no payment guide for providers without a flow guide', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.alipayGuideSummary'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.wxpayGuideSummary'])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(false)
  REDACTED)

  it.each([
    ['alipay', 'admin.settings.payment.alipayGuideSummary'],
    ['wxpay', 'admin.settings.payment.wxpayGuideSummary'],
    ['airwallex', 'admin.settings.payment.airwallexGuideSummary'],
  ])('shows the payment guide summary for %s', async (providerKey, summaryKey) => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void REDACTED).reset(providerKey)
    await nextTick()

    expect(wrapper.text()).toContain(messages[summaryKey])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(true)
  REDACTED)

  it('shows Airwallex webhook event and API version guidance with the webhook URL', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void REDACTED).reset('airwallex')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.airwallexWebhookHint'])
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/airwallex')
  REDACTED)

  it('shows Stripe webhook API version guidance with the integrated SDK version', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void REDACTED).reset('stripe')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.stripeWebhookHint'])
    expect(wrapper.text()).toContain(`Use Stripe API version ${STRIPE_SDK_API_VERSIONREDACTED.`)
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/stripe')
  REDACTED)

  it('emits an empty Airwallex accountId when the admin clears it', async () => {
    const provider = providerFactory({
      config: {
        clientId: 'cid_123',
        apiBase: 'https://api.airwallex.com/api/v1',
        countryCode: 'CN',
        currency: 'CNY',
        accountId: 'acct_123',
      REDACTED,
    REDACTED)
    const wrapper = mountDialog({ editing: provider REDACTED)

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void REDACTED).loadProvider(provider)
    await nextTick()

    const accountIdInput = wrapper
      .findAll('input[type="text"]')
      .find(input => (input.element as HTMLInputElement).value === 'acct_123')
    if (!accountIdInput) throw new Error('accountId input not found')

    await accountIdInput.setValue('')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> REDACTED
    expect(payload.config.accountId).toBe('')
  REDACTED)

  it('serializes EasyPay custom methods and adds them to supported_types', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      REDACTED,
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    REDACTED)
    const wrapper = mountDialog({ editing: provider REDACTED)

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void REDACTED).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const ldcTypeInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'ldc')
    const upstreamTypeInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'epay')
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'LDC')
    if (!ldcTypeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    REDACTED

    await ldcTypeInput.setValue('ldc')
    await upstreamTypeInput.setValue('epay')
    await displayNameInput.setValue('LDC')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as {
      config: Record<string, string>
      supported_types: string[]
    REDACTED
    expect(payload.config.customMethods).toBe('[{"type":"ldc","upstreamType":"epay","displayName":"LDC"REDACTED]')
    expect(payload.supported_types).toEqual(['alipay', 'wxpay', 'ldc'])
  REDACTED)

  it('rejects custom EasyPay method types with built-in payment prefixes', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      REDACTED,
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    REDACTED)
    const wrapper = mountDialog({ editing: provider REDACTED)

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void REDACTED).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const typeInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'ldc')
    const upstreamTypeInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'epay')
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === 'LDC')
    if (!typeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    REDACTED

    await typeInput.setValue('alipay_hk')
    await upstreamTypeInput.setValue('hkpay')
    await displayNameInput.setValue('Hong Kong Alipay')
    await wrapper.find('form').trigger('submit.prevent')

    expect(wrapper.emitted('save')).toBeUndefined()
  REDACTED)
REDACTED)
