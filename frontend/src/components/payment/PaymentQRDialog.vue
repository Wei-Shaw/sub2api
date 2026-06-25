<template>
  <BaseDialog :show="show" :title="dialogTitle" width="narrow" @close="handleClose">
    <!-- QR Code + Polling State -->
    <div v-if="!success" class="flex flex-col items-center space-y-4">
      <!-- QR Code mode -->
      <template v-if="qrUrl">
        <div class="rounded-2xl bg-white p-4 shadow-sm dark:bg-dark-800">
          <canvas ref="qrCanvas" class="mx-auto"></canvas>
        </div>
        <p v-if="scanHint" class="text-center text-sm text-gray-500 dark:text-gray-400">
          {{ scanHint REDACTEDREDACTED
        </p>
      </template>
      <!-- Popup window waiting mode (no QR code) -->
      <template v-else>
        <div class="flex flex-col items-center py-4">
          <div class="h-10 w-10 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
          <p class="mt-4 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.payInNewWindowHint') REDACTEDREDACTED</p>
          <button v-if="payUrl" class="btn btn-secondary mt-3 text-sm" @click="reopenPopup">
            {{ t('payment.qr.openPayWindow') REDACTEDREDACTED
          </button>
        </div>
      </template>
      <!-- Countdown -->
      <div v-if="expired" class="text-center">
        <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') REDACTEDREDACTED</p>
      </div>
      <div v-else class="text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ qrUrl ? t('payment.qr.expiresIn') : '' REDACTEDREDACTED</p>
        <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay REDACTEDREDACTED</p>
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') REDACTEDREDACTED</p>
      </div>
    </div>
    <!-- Success State -->
    <div v-else class="flex flex-col items-center space-y-4 py-4">
      <div class="flex h-16 w-16 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
        <Icon name="check" size="lg" class="text-green-500" />
      </div>
      <p class="text-lg font-bold text-gray-900 dark:text-white">{{ t('payment.result.success') REDACTEDREDACTED</p>
      <div v-if="paidOrder" class="w-full rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') REDACTEDREDACTED</span>
            <span class="font-medium text-gray-900 dark:text-white">#{{ paidOrder.id REDACTEDREDACTED</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') REDACTEDREDACTED</span>
            <span class="font-medium text-gray-900 dark:text-white">{{ creditedAmountSymbol REDACTEDREDACTED{{ paidOrder.amount.toFixed(2) REDACTEDREDACTED</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') REDACTEDREDACTED</span>
            <span class="font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(paidOrder) REDACTEDREDACTED{{ paidOrder.pay_amount.toFixed(2) REDACTEDREDACTED</span>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button v-if="!success && !expired" class="btn btn-secondary" :disabled="cancelling" @click="handleCancel">
          {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') REDACTEDREDACTED
        </button>
        <button v-if="success" class="btn btn-primary" @click="handleDone">
          {{ t('common.confirm') REDACTEDREDACTED
        </button>
        <button v-if="expired" class="btn btn-primary" @click="handleClose">
          {{ t('payment.result.backToRecharge') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { usePaymentStore REDACTED from '@/stores/payment'
import { useAppStore REDACTED from '@/stores'
import { paymentAPI REDACTED from '@/api/payment'
import { extractI18nErrorMessage REDACTED from '@/utils/apiError'
import { getPaymentPopupFeatures REDACTED from '@/components/payment/providerConfig'
import type { PaymentOrder REDACTED from '@/types/payment'
import { currencySymbol REDACTED from '@/components/payment/currency'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'

const props = defineProps<{
  show: boolean
  orderId: number
  qrCode: string
  expiresAt: string
  paymentType: string
  /** URL for reopening the payment popup window */
  payUrl?: string
REDACTED>()

const emit = defineEmits<{
  close: []
  success: []
REDACTED>()

const { t REDACTED = useI18n()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const expired = ref(false)
const cancelling = ref(false)
const success = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)
const creditedAmountSymbol = currencySymbol('USD')

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null
let verifyAttempts = 0
let lastVerifyAt = 0

const VERIFY_RETRY_INTERVAL_MS = 15000
const VERIFY_RETRY_MAX_ATTEMPTS = 6

const isAlipay = computed(() => props.paymentType.includes('alipay'))
const isWxpay = computed(() => props.paymentType.includes('wxpay'))

const dialogTitle = computed(() => {
  if (success.value) return t('payment.result.success')
  if (!qrUrl.value) return t('payment.qr.payInNewWindow')
  if (isAlipay.value) return t('payment.qr.scanAlipay')
  if (isWxpay.value) return t('payment.qr.scanWxpay')
  return t('payment.qr.scanToPay')
REDACTED)

const scanHint = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipayHint')
  if (isWxpay.value) return t('payment.qr.scanWxpayHint')
  return ''
REDACTED)

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
REDACTED

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
REDACTED)

function getLogoForType(): string | null {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return null
REDACTED


function reopenPopup() {
  if (props.payUrl) {
    window.open(props.payUrl, 'paymentPopup', getPaymentPopupFeatures())
  REDACTED
REDACTED

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return
  const logoSrc = getLogoForType()
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220,
    margin: 2,
    errorCorrectionLevel: logoSrc ? 'M' : 'L',
  REDACTED)
  if (!logoSrc) return
  const canvas = qrCanvas.value
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  const img = new Image()
  img.src = logoSrc
  img.onload = () => {
    const logoSize = 40
    const x = (canvas.width - logoSize) / 2
    const y = (canvas.height - logoSize) / 2
    const pad = 4
    ctx.fillStyle = '#FFFFFF'
    ctx.beginPath()
    const r = 5
    ctx.moveTo(x - pad + r, y - pad)
    ctx.arcTo(x + logoSize + pad, y - pad, x + logoSize + pad, y + logoSize + pad, r)
    ctx.arcTo(x + logoSize + pad, y + logoSize + pad, x - pad, y + logoSize + pad, r)
    ctx.arcTo(x - pad, y + logoSize + pad, x - pad, y - pad, r)
    ctx.arcTo(x - pad, y - pad, x + logoSize + pad, y - pad, r)
    ctx.fill()
    ctx.drawImage(img, x, y, logoSize, logoSize)
  REDACTED
REDACTED

async function pollStatus() {
  if (!props.orderId) return
  let order = await paymentStore.pollOrderStatus(props.orderId)
  if (!order) return
  order = await tryRecoverPendingOrder(order)
  if (order.status === 'COMPLETED' || order.status === 'PAID') {
    cleanup()
    paidOrder.value = order
    success.value = true
    emit('success')
  REDACTED else if (order.status === 'EXPIRED' || order.status === 'CANCELLED' || order.status === 'FAILED') {
    cleanup()
    expired.value = true
  REDACTED
REDACTED

async function tryRecoverPendingOrder(order: PaymentOrder): Promise<PaymentOrder> {
  if (!isWxpay.value) return order
  const outTradeNo = String(order.out_trade_no || '').trim()
  if (!outTradeNo) return order
  const normalizedStatus = String(order.status || '').trim().toUpperCase()
  if (normalizedStatus !== 'PENDING') return order
  const now = Date.now()
  if (verifyAttempts >= VERIFY_RETRY_MAX_ATTEMPTS || now - lastVerifyAt < VERIFY_RETRY_INTERVAL_MS) {
    return order
  REDACTED

  lastVerifyAt = now
  verifyAttempts += 1
  try {
    const result = await paymentAPI.verifyOrder(outTradeNo)
    return result.data ?? order
  REDACTED catch {
    return order
  REDACTED
REDACTED

function startCountdown(seconds: number) {
  remainingSeconds.value = Math.max(0, seconds)
  if (remainingSeconds.value <= 0) {
    expired.value = true
    return
  REDACTED
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      expired.value = true
      cleanup()
    REDACTED
  REDACTED, 1000)
REDACTED

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    emit('close')
  REDACTED catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  REDACTED finally {
    cancelling.value = false
  REDACTED
REDACTED

function handleClose() {
  cleanup()
  emit('close')
REDACTED

function handleDone() {
  cleanup()
  emit('close')
REDACTED

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null REDACTED
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null REDACTED
REDACTED

function init() {
  // Reset state
  success.value = false
  paidOrder.value = null
  expired.value = false
  cancelling.value = false
  qrUrl.value = props.qrCode
  verifyAttempts = 0
  lastVerifyAt = 0

  let seconds = 30 * 60
  if (props.expiresAt) {
    const expiresAt = new Date(props.expiresAt)
    seconds = Math.floor((expiresAt.getTime() - Date.now()) / 1000)
  REDACTED
  startCountdown(seconds)
  pollTimer = setInterval(pollStatus, 3000)
  renderQR()
REDACTED

// Watch for dialog open/close
watch(() => props.show, (isOpen) => {
  if (isOpen) {
    init()
  REDACTED else {
    cleanup()
  REDACTED
REDACTED)

watch(qrUrl, () => renderQR())

onUnmounted(() => cleanup())
</script>
