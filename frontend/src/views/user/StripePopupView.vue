<template>
  <div class="flex min-h-screen items-center justify-center bg-slate-50 p-4 dark:bg-slate-950">
    <div
      class="w-full max-w-md space-y-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-lg dark:border-slate-700 dark:bg-slate-900"
    >
      <!-- Amount + Order ID -->
      <div v-if="amount" class="text-center">
        <p class="text-3xl font-bold" :style="{ color: methodColor REDACTED">¥{{ amount REDACTEDREDACTED</p>
        <p v-if="orderId" class="mt-1 text-sm text-gray-500 dark:text-slate-400">
          {{ t('payment.orders.orderId') REDACTEDREDACTED: {{ orderId REDACTEDREDACTED
        </p>
      </div>

      <!-- Error -->
      <div v-if="error" class="space-y-3">
        <div
          class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-600 dark:border-red-700 dark:bg-red-900/30 dark:text-red-400"
        >
          {{ error REDACTEDREDACTED
        </div>
        <button
          class="w-full text-sm underline dark:text-blue-400 dark:hover:text-blue-300"
          :style="{ color: methodColor REDACTED"
          @click="closeWindow"
        >
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>

      <!-- Success -->
      <div v-else-if="success" class="space-y-3 py-4 text-center">
        <div class="text-5xl text-green-600 dark:text-green-400">✓</div>
        <p class="text-sm text-gray-500 dark:text-slate-400">{{ t('payment.result.success') REDACTEDREDACTED</p>
        <button
          class="text-sm underline dark:text-blue-400 dark:hover:text-blue-300"
          :style="{ color: methodColor REDACTED"
          @click="closeWindow"
        >
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>

      <!-- Loading / Redirecting -->
      <div v-else class="flex items-center justify-center py-8">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-t-transparent"
          :style="{ borderColor: methodColor, borderTopColor: 'transparent' REDACTED"
        />
        <span class="ml-3 text-sm text-gray-500 dark:text-slate-400">{{ hint REDACTEDREDACTED</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useRoute REDACTED from 'vue-router'
import { extractI18nErrorMessage REDACTED from '@/utils/apiError'
import { isMobileDevice REDACTED from '@/utils/device'
import { buildApiUrl REDACTED from '@/api/client'

interface StripeWithWechatPay {
  confirmWechatPayPayment(clientSecret: string, options: Record<string, unknown>): Promise<{ error?: { message?: string REDACTED; paymentIntent?: { status: string REDACTED REDACTED>
REDACTED

const METHOD_COLORS: Record<string, string> = {
  alipay: '#00AEEF',
  wechat_pay: '#07C160',
REDACTED
const DEFAULT_METHOD_COLOR = '#635bff'

const { t REDACTED = useI18n()
const route = useRoute()

const orderId = String(route.query.order_id || '')
const method = String(route.query.method || 'alipay')
const amount = String(route.query.amount || '')

const methodColor = computed(() => METHOD_COLORS[method] || DEFAULT_METHOD_COLOR)

const error = ref('')
const success = ref(false)
const hint = ref(t('payment.stripePopup.redirecting'))

let pollTimer: ReturnType<typeof setInterval> | null = null

function closeWindow() { window.close() REDACTED

onMounted(() => {
  const handler = (event: MessageEvent) => {
    if (event.origin !== window.location.origin) return
    if (event.data?.type !== 'STRIPE_POPUP_INIT') return
    window.removeEventListener('message', handler)
    initStripe(event.data.clientSecret, event.data.publishableKey)
  REDACTED
  window.addEventListener('message', handler)

  if (window.opener) {
    window.opener.postMessage({ type: 'STRIPE_POPUP_READY' REDACTED, window.location.origin)
  REDACTED

  setTimeout(() => {
    if (!error.value && !success.value) {
      error.value = t('payment.stripePopup.timeout')
    REDACTED
  REDACTED, 15000)
REDACTED)

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
REDACTED)

async function initStripe(clientSecret: string, publishableKey: string) {
  if (!clientSecret || !publishableKey) {
    error.value = t('payment.stripeMissingParams')
    return
  REDACTED
  try {
    const { loadStripe REDACTED = await import('@stripe/stripe-js')
    const stripe = await loadStripe(publishableKey)
    if (!stripe) { error.value = t('payment.stripeLoadFailed'); return REDACTED

    const returnUrl = window.location.origin + '/payment/result?order_id=' + orderId + '&status=success'

    if (method === 'alipay') {
      // Alipay: redirect this popup to Alipay payment page
      const { error: err REDACTED = await stripe.confirmAlipayPayment(clientSecret, { return_url: returnUrl REDACTED)
      if (err) error.value = err.message || t('payment.result.failed')
    REDACTED else if (method === 'wechat_pay') {
      // WeChat: Stripe shows its built-in QR dialog, user scans, promise resolves
      hint.value = t('payment.stripePopup.loadingQr')
      const result = await (stripe as unknown as StripeWithWechatPay).confirmWechatPayPayment(clientSecret, {
        payment_method_options: { wechat_pay: { client: isMobileDevice() ? 'mobile_web' : 'web' REDACTED REDACTED,
      REDACTED)
      if (result.error) {
        error.value = result.error.message || t('payment.result.failed')
      REDACTED else if (result.paymentIntent?.status === 'succeeded') {
        success.value = true
        setTimeout(closeWindow, 2000)
      REDACTED else {
        // Payment not completed (user closed QR dialog)
        startPolling()
      REDACTED
    REDACTED
  REDACTED catch (err: unknown) {
    error.value = extractI18nErrorMessage(err, t, 'payment.errors', t('payment.stripeLoadFailed'))
  REDACTED
REDACTED

function startPolling() {
  pollTimer = setInterval(async () => {
    try {
      const token = document.cookie.split('; ').find(c => c.startsWith('token='))?.split('=')[1]
        || localStorage.getItem('token') || ''
      const res = await fetch(buildApiUrl(`/payment/orders/${orderIdREDACTED`), {
        headers: token ? { Authorization: 'Bearer ' + token REDACTED : {REDACTED,
        credentials: 'include',
      REDACTED)
      if (!res.ok) return
      const data = await res.json()
      const status = data?.data?.status
      if (status === 'COMPLETED' || status === 'PAID') {
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null REDACTED
        success.value = true
        setTimeout(closeWindow, 2000)
      REDACTED
    REDACTED catch { /* ignore */ REDACTED
  REDACTED, 3000)
REDACTED
</script>
