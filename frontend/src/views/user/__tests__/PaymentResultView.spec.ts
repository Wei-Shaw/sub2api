import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  query: {REDACTED as Record<string, unknown>,
REDACTED))

const routerPush = vi.hoisted(() => vi.fn())
const pollOrderStatus = vi.hoisted(() => vi.fn())
const verifyOrder = vi.hoisted(() => vi.fn())
const verifyOrderPublic = vi.hoisted(() => vi.fn())
const resolveOrderPublicByResumeToken = vi.hoisted(() => vi.fn())

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
    REDACTED),
  REDACTED
REDACTED)

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    pollOrderStatus,
  REDACTED),
REDACTED))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    verifyOrder,
    verifyOrderPublic,
    resolveOrderPublicByResumeToken,
  REDACTED,
REDACTED))

import PaymentResultView from '../PaymentResultView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY REDACTED from '@/components/payment/paymentFlow'
import { formatPaymentAmount REDACTED from '@/components/payment/currency'

const orderFactory = (status: string) => ({
  id: 42,
  user_id: 9,
  amount: 88,
  pay_amount: 88,
  fee_rate: 0,
  payment_type: 'alipay',
  out_trade_no: 'sub2_20260420abcd1234',
  status,
  order_type: 'balance',
  created_at: '2026-04-20T12:00:00Z',
  expires_at: '2026-04-20T12:30:00Z',
  refund_amount: 0,
REDACTED)

const recoverySnapshotFactory = (resumeToken: string) => ({
  orderId: 42,
  amount: 88,
  qrCode: '',
  expiresAt: '2099-01-01T00:10:00.000Z',
  paymentType: 'alipay',
  payUrl: 'https://pay.example.com/session/42',
  outTradeNo: 'sub2_20260420abcd1234',
  clientSecret: '',
  intentId: '',
  currency: '',
  countryCode: '',
  paymentEnv: '',
  payAmount: 88,
  orderType: 'balance',
  paymentMode: 'popup',
  resumeToken,
  createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
REDACTED)

describe('PaymentResultView', () => {
  beforeEach(() => {
    routeState.query = {REDACTED
    routerPush.mockReset()
    pollOrderStatus.mockReset()
    verifyOrder.mockReset()
    verifyOrderPublic.mockReset()
    resolveOrderPublicByResumeToken.mockReset()
    window.localStorage.clear()
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  it('renders a pending state instead of a failure state when the restored order is still pending', async () => {
    routeState.query = {
      resume_token: 'resume-42',
      order_id: '999',
      status: 'success',
    REDACTED
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 42,
      amount: 88,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/42',
      outTradeNo: 'sub2_20260420abcd1234',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 88,
      orderType: 'balance',
      paymentMode: 'redirect',
      resumeToken: 'resume-42',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    REDACTED))
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: orderFactory('PENDING'),
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-42')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(wrapper.text()).not.toContain('payment.result.success')
    expect(wrapper.text()).not.toContain('payment.result.failed')
  REDACTED)

  it('prefers the public resume-token result over a stale restored DB snapshot', async () => {
    routeState.query = {
      resume_token: 'resume-authoritative',
      order_id: '42',
      status: 'success',
    REDACTED
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 42,
      amount: 88,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/session/42',
      outTradeNo: 'sub2_20260420abcd1234',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 88,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: 'resume-authoritative',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    REDACTED))
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: {
        ...orderFactory('PAID'),
        amount: 100,
        pay_amount: 103,
        fee_rate: 3,
      REDACTED,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-authoritative')
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).toContain('103.00')
    expect(wrapper.text()).toContain('100.00')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  REDACTED)

  it('refreshes a pending resume-token result until the order becomes paid', async () => {
    vi.useFakeTimers()
    routeState.query = {
      resume_token: 'resume-77',
    REDACTED
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify(recoverySnapshotFactory('resume-77')),
    )
    resolveOrderPublicByResumeToken
      .mockResolvedValueOnce({
        data: orderFactory('PENDING'),
      REDACTED)
      .mockResolvedValueOnce({
        data: orderFactory('PAID'),
      REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('payment.result.processing')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).not.toBeNull()

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).not.toContain('payment.result.failed')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  REDACTED)

  it('falls back to order_id polling when resume-token recovery fails', async () => {
    routeState.query = {
      resume_token: 'resume-fail',
      order_id: '77',
    REDACTED
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify({
        ...recoverySnapshotFactory('resume-fail'),
        orderId: 42,
      REDACTED),
    )
    resolveOrderPublicByResumeToken.mockRejectedValueOnce(new Error('resume failed'))
    pollOrderStatus.mockResolvedValueOnce({
      ...orderFactory('PAID'),
      id: 77,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-fail')
    expect(pollOrderStatus).toHaveBeenCalledWith(77)
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.success')
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  REDACTED)

  it('falls back to public out_trade_no verification when resume_token recovery fails in legacy return flows', async () => {
    routeState.query = {
      resume_token: 'resume-fail',
      out_trade_no: 'legacy-should-not-run',
      trade_status: 'TRADE_SUCCESS',
    REDACTED
    resolveOrderPublicByResumeToken.mockRejectedValueOnce(new Error('resume failed'))
    verifyOrderPublic.mockResolvedValueOnce({
      data: {
        ...orderFactory('PAID'),
        out_trade_no: 'legacy-should-not-run',
      REDACTED,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-fail')
    expect(verifyOrderPublic).toHaveBeenCalledWith('legacy-should-not-run')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.success')
  REDACTED)

  it('ignores a stale global recovery snapshot when legacy return markers do not identify the order', async () => {
    routeState.query = {
      trade_status: 'TRADE_SUCCESS',
    REDACTED
    window.localStorage.setItem(
      PAYMENT_RECOVERY_STORAGE_KEY,
      JSON.stringify(recoverySnapshotFactory('resume-stale')),
    )

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).not.toHaveBeenCalled()
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.failed')
    expect(wrapper.text()).not.toContain('sub2_20260420abcd1234')
  REDACTED)

  it('uses public out_trade_no verification when no signed resume context is available', async () => {
    routeState.query = {
      out_trade_no: 'legacy-123',
      trade_status: 'TRADE_SUCCESS',
    REDACTED
    verifyOrder.mockRejectedValue(new Error('auth required'))
    verifyOrderPublic.mockResolvedValue({
      data: orderFactory('PAID'),
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(verifyOrder).toHaveBeenCalledWith('legacy-123')
    expect(verifyOrderPublic).toHaveBeenCalledWith('legacy-123')
    expect(pollOrderStatus).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.success')
  REDACTED)

  it('renders the minimal public out_trade_no verification result without payment_type', async () => {
    routeState.query = {
      out_trade_no: 'legacy-minimal',
      trade_status: 'TRADE_SUCCESS',
    REDACTED
    verifyOrder.mockRejectedValue(new Error('auth required'))
    verifyOrderPublic.mockResolvedValue({
      data: {
        out_trade_no: 'legacy-minimal',
        status: 'PAID',
        paid: true,
        created_at: '2026-04-20T12:00:00Z',
        expires_at: '2026-04-20T12:30:00Z',
      REDACTED,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('payment.result.success')
    expect(wrapper.text()).toContain('legacy-minimal')
    expect(wrapper.text()).not.toContain('payment.orders.paymentMethod')
  REDACTED)

  it('prefers authenticated order verification before falling back to public lookup', async () => {
    routeState.query = {
      out_trade_no: 'auth-verify-123',
      trade_status: 'TRADE_SUCCESS',
    REDACTED
    verifyOrder.mockResolvedValue({
      data: orderFactory('COMPLETED'),
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(verifyOrder).toHaveBeenCalledWith('auth-verify-123')
    expect(verifyOrderPublic).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('payment.result.success')
  REDACTED)

  it('does not use public out_trade_no verification for bare order numbers without legacy return markers', async () => {
    routeState.query = {
      out_trade_no: 'legacy-bare',
    REDACTED

    mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(verifyOrderPublic).not.toHaveBeenCalled()
  REDACTED)

  it('resolves order by resume token when local recovery snapshot is missing', async () => {
    routeState.query = {
      resume_token: 'resume-77',
    REDACTED
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: orderFactory('PAID'),
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-77')
    expect(wrapper.text()).toContain('payment.result.success')
  REDACTED)

  it('uses the currency returned by the order API when rendering amounts', async () => {
    routeState.query = {
      resume_token: 'resume-hkd',
    REDACTED
    resolveOrderPublicByResumeToken.mockResolvedValue({
      data: {
        ...orderFactory('PAID'),
        currency: 'HKD',
        amount: 100,
        pay_amount: 103,
        fee_rate: 3,
      REDACTED,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain(formatPaymentAmount(103, 'HKD'))
  REDACTED)

  it('normalizes aliased payment methods before rendering the label', async () => {
    routeState.query = {
      resume_token: 'resume-88',
    REDACTED
    resolveOrderPublicByResumeToken.mockResolvedValueOnce({
      data: {
        ...orderFactory('PAID'),
        payment_type: 'alipay_direct',
      REDACTED,
    REDACTED)

    const wrapper = mount(PaymentResultView, {
      global: {
        stubs: {
          OrderStatusBadge: true,
        REDACTED,
      REDACTED,
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('payment.methods.alipay')
    expect(wrapper.text()).not.toContain('payment.methods.alipay_direct')
  REDACTED)
REDACTED)
