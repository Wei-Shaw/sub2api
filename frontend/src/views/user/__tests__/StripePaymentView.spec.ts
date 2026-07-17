import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, shallowMount REDACTED from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>,
REDACTED))
const routerPush = vi.hoisted(() => vi.fn())
const getOrder = vi.hoisted(() => vi.fn())
const paymentStore = vi.hoisted(() => ({
  config: { stripe_publishable_key: 'pk_test' REDACTED as { stripe_publishable_key?: string REDACTED,
  fetchConfig: vi.fn(),
  pollOrderStatus: vi.fn(),
REDACTED))
const loadStripe = vi.hoisted(() => vi.fn())
const stripeElements = vi.hoisted(() => ({
  create: vi.fn(),
REDACTED))
const stripePaymentElement = vi.hoisted(() => ({
  mount: vi.fn(),
  on: vi.fn(),
REDACTED))
const stripeInstance = vi.hoisted(() => ({
  elements: vi.fn(),
  confirmPayment: vi.fn(),
  confirmAlipayPayment: vi.fn(),
  confirmWechatPayPayment: vi.fn(),
REDACTED))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({ push: routerPush REDACTED),
  REDACTED
REDACTED)

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' REDACTED,
    REDACTED),
  REDACTED
REDACTED)

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => paymentStore,
REDACTED))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getOrder,
  REDACTED,
REDACTED))

vi.mock('@stripe/stripe-js/pure', () => ({
  loadStripe,
REDACTED))

import StripePaymentView from '../StripePaymentView.vue'
import { formatPaymentAmount REDACTED from '@/components/payment/currency'
import type { PaymentOrder REDACTED from '@/types/payment'

function orderFactory(overrides: Partial<PaymentOrder> = {REDACTED): PaymentOrder {
  return {
    id: 42,
    user_id: 7,
    amount: 100,
    pay_amount: 103,
    currency: 'CNY',
    fee_rate: 0.03,
    payment_type: 'stripe',
    out_trade_no: 'sub2_stripe_42',
    status: 'PENDING',
    order_type: 'balance',
    created_at: '2026-04-20T12:00:00Z',
    expires_at: '2026-04-20T12:30:00Z',
    refund_amount: 0,
    ...overrides,
  REDACTED
REDACTED

function mountView() {
  return shallowMount(StripePaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' REDACTED,
        Icon: true,
      REDACTED,
    REDACTED,
  REDACTED)
REDACTED

describe('StripePaymentView', () => {
  beforeEach(() => {
    routeState.query = {
      order_id: '42',
      client_secret: 'pi_secret_42',
    REDACTED
    routerPush.mockReset()
    getOrder.mockReset()
    paymentStore.config = { stripe_publishable_key: 'pk_test' REDACTED
    paymentStore.fetchConfig.mockReset().mockResolvedValue(undefined)
    paymentStore.pollOrderStatus.mockReset()
    loadStripe.mockReset().mockResolvedValue(stripeInstance)
    stripeElements.create.mockReset().mockReturnValue(stripePaymentElement)
    stripePaymentElement.mount.mockReset()
    stripePaymentElement.on.mockReset().mockImplementation((event: string, callback: () => void) => {
      if (event === 'ready') callback()
    REDACTED)
    stripeInstance.elements.mockReset().mockReturnValue(stripeElements)
    stripeInstance.confirmPayment.mockReset()
    stripeInstance.confirmAlipayPayment.mockReset()
    stripeInstance.confirmWechatPayPayment.mockReset()
    window.localStorage.clear()
  REDACTED)

  it('本地恢复快照缺失时使用订单接口返回的 Stripe 币种展示金额', async () => {
    getOrder.mockResolvedValue({
      data: orderFactory({ currency: 'HKD', pay_amount: 103 REDACTED),
    REDACTED)

    const wrapper = mountView()
    await flushPromises()
    await flushPromises()

    expect(getOrder).toHaveBeenCalledWith(42)
    expect(loadStripe).toHaveBeenCalledWith('pk_test')
    expect(wrapper.text()).toContain(formatPaymentAmount(103, 'HKD', 'zh-CN'))
  REDACTED)
REDACTED)
