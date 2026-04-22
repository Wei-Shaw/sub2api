import { describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import { nextTick REDACTED from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.paymentGuideTrigger': 'View payment guide',
  'admin.settings.payment.alipayGuideSummary': 'Desktop prefers QR precreate and falls back to cashier; mobile prefers WAP checkout.',
  'admin.settings.payment.wxpayGuideSummary': 'Desktop prefers Native QR; mobile routes to JSAPI or H5 based on browser context.',
REDACTED

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
  REDACTED),
REDACTED))

function mountDialog() {
  return mount(PaymentProviderDialog, {
    props: {
      show: true,
      saving: false,
      editing: null,
      allKeyOptions: [
        { value: 'alipay', label: 'Alipay' REDACTED,
        { value: 'wxpay', label: 'WeChat Pay' REDACTED,
        { value: 'stripe', label: 'Stripe' REDACTED,
      ],
      enabledKeyOptions: [
        { value: 'alipay', label: 'Alipay' REDACTED,
        { value: 'wxpay', label: 'WeChat Pay' REDACTED,
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
  ])('shows the payment guide summary for %s', async (providerKey, summaryKey) => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void REDACTED).reset(providerKey)
    await nextTick()

    expect(wrapper.text()).toContain(messages[summaryKey])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(true)
  REDACTED)
REDACTED)
