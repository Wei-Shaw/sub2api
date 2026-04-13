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
          {{ scanHint }}
        </p>
      </template>
      <!-- Popup window waiting mode (no QR code) -->
      <template v-else>
        <div class="flex flex-col items-center py-4">
          <div class="h-10 w-10 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
          <p class="mt-4 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.qr.payInNewWindowHint') }}</p>
          <button v-if="payUrl" class="btn btn-secondary mt-3 text-sm" @click="reopenPopup">
            {{ t('payment.qr.openPayWindow') }}
          </button>
        </div>
      </template>
      <!-- Countdown -->
      <div v-if="expired" class="text-center">
        <p class="text-lg font-medium text-red-500">{{ t('payment.qr.expired') }}</p>
      </div>
      <div v-else class="text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ qrUrl ? t('payment.qr.expiresIn') : '' }}</p>
        <p class="mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</p>
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{{ t('payment.qr.waitingPayment') }}</p>
      </div>
    </div>
    <!-- Success State -->
    <div v-else class="flex flex-col items-center space-y-4 py-4">
      <div class="flex h-16 w-16 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
        <Icon name="check" size="lg" class="text-green-500" />
      </div>
      <p class="text-lg font-bold text-gray-900 dark:text-white">{{ t('payment.result.success') }}</p>
      <div v-if="paidOrder" class="w-full rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</span>
            <span class="font-medium text-gray-900 dark:text-white">#{{ paidOrder.id }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</span>
            <span class="font-medium text-gray-900 dark:text-white">${{ paidOrder.pay_amount.toFixed(2) }}</span>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button v-if="!success && !expired" class="btn btn-secondary" :disabled="cancelling" @click="handleCancel">
          {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
        </button>
        <button v-if="success" class="btn btn-primary" @click="handleDone">
          {{ t('common.confirm') }}
        </button>
        <button v-if="expired" class="btn btn-primary" @click="handleClose">
          {{ t('payment.result.backToRecharge') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { POPUP_WINDOW_FEATURES } from '@/components/payment/providerConfig'
import type { PaymentOrder } from '@/types/payment'
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
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const paymentStore = usePaymentStore()
const appStore = useAppStore()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const expired = ref(false)
const cancelling = ref(false)
const success = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null

const isAlipay = computed(() => props.paymentType.includes('alipay'))
const isWxpay = computed(() => props.paymentType.includes('wxpay'))

const dialogTitle = computed(() => {
  if (success.value) return t('payment.result.success')
  if (!qrUrl.value) return t('payment.qr.payInNewWindow')
  if (isAlipay.value) return t('payment.qr.scanAlipay')
  if (isWxpay.value) return t('payment.qr.scanWxpay')
  return t('payment.qr.scanToPay')
})

const scanHint = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipayHint')
  if (isWxpay.value) return t('payment.qr.scanWxpayHint')
  return ''
})

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

function getLogoForType(): string | null {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return null
}


function reopenPopup() {
  if (props.payUrl) {
    window.open(props.payUrl, 'paymentPopup', POPUP_WINDOW_FEATURES)
  }
}

async function renderQR() {
  await nextTick()
  if (!qrCanvas.value || !qrUrl.value) return
  const logoSrc = getLogoForType()
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220,
    margin: 2,
    errorCorrectionLevel: logoSrc ? 'M' : 'L',
  })
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
  }
}

async function pollStatus() {
  if (!props.orderId) return
  const order = await paymentStore.pollOrderStatus(props.orderId)
  if (!order) return
  if (order.status === 'COMPLETED' || order.status === 'PAID') {
    cleanup()
    paidOrder.value = order
    success.value = true
    emit('success')
  } else if (order.status === 'EXPIRED' || order.status === 'CANCELLED' || order.status === 'FAILED') {
    cleanup()
    expired.value = true
  }
}

function startCountdown(seconds: number) {
  remainingSeconds.value = Math.max(0, seconds)
  if (remainingSeconds.value <= 0) {
    expired.value = true
    return
  }
  countdownTimer = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      expired.value = true
      cleanup()
    }
  }, 1000)
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    cancelling.value = false
  }
}

function handleClose() {
  cleanup()
  emit('close')
}

function handleDone() {
  cleanup()
  emit('close')
}

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
}

function init() {
  // Reset state
  success.value = false
  paidOrder.value = null
  expired.value = false
  cancelling.value = false
  qrUrl.value = props.qrCode

  let seconds = 30 * 60
  if (props.expiresAt) {
    const expiresAt = new Date(props.expiresAt)
    seconds = Math.floor((expiresAt.getTime() - Date.now()) / 1000)
  }
  startCountdown(seconds)
  pollTimer = setInterval(pollStatus, 3000)
  renderQR()
}

// Watch for dialog open/close
watch(() => props.show, (isOpen) => {
  if (isOpen) {
    init()
  } else {
    cleanup()
  }
})

watch(qrUrl, () => renderQR())

onUnmounted(() => cleanup())
</script>
